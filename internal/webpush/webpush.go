// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package webpush sends encrypted messages to browsers through their own push
// services, as defined by RFC 8030 (delivery), RFC 8291 (encryption) and
// RFC 8292 (VAPID server identification).
package webpush // import "miniflux.app/v2/internal/webpush"

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"miniflux.app/v2/internal/config"
	"miniflux.app/v2/internal/http/client"
)

const (
	contactURL     = "https://github.com/laurens-pilot/maxifeed"
	recordSize     = 4096
	saltSize       = 16
	tokenLifetime  = 12 * time.Hour
	messageTTL     = 6 * time.Hour
	requestTimeout = 15 * time.Second
	maxErrorBody   = 512
)

// ErrSubscriptionGone is returned when the push service no longer knows the
// subscription, which means the device unsubscribed or the browser was removed.
var ErrSubscriptionGone = errors.New("webpush: the subscription no longer exists")

// Subscription identifies a browser on a device.
type Subscription struct {
	Endpoint string
	P256DH   string
	Auth     string
}

// GenerateKey returns a new raw VAPID private key.
func GenerateKey() ([]byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("webpush: unable to generate a key: %w", err)
	}

	return key.Bytes()
}

// PublicKey returns the base64url public key that browsers need to subscribe.
func PublicKey(privateKey []byte) (string, error) {
	key, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), privateKey)
	if err != nil {
		return "", fmt.Errorf("webpush: invalid private key: %w", err)
	}

	publicKey, err := key.PublicKey.Bytes()
	if err != nil {
		return "", fmt.Errorf("webpush: invalid public key: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(publicKey), nil
}

// Send encrypts the payload for the subscription and hands it to its push service.
func Send(privateKey []byte, subscription *Subscription, payload []byte) error {
	request, err := newRequest(privateKey, subscription, payload, time.Now())
	if err != nil {
		return err
	}

	httpClient := client.NewClientWithOptions(client.Options{
		Timeout:              requestTimeout,
		BlockPrivateNetworks: !config.Opts.IntegrationAllowPrivateNetworks(),
	})
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	response, err := httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("webpush: unable to reach the push service: %w", err)
	}
	defer response.Body.Close()

	return checkResponse(response)
}

func newRequest(privateKey []byte, subscription *Subscription, payload []byte, now time.Time) (*http.Request, error) {
	endpoint, err := url.Parse(subscription.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("webpush: invalid endpoint: %w", err)
	}

	signingKey, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), privateKey)
	if err != nil {
		return nil, fmt.Errorf("webpush: invalid private key: %w", err)
	}

	publicKey, err := PublicKey(privateKey)
	if err != nil {
		return nil, err
	}

	token, err := newToken(signingKey, endpoint.Scheme+"://"+endpoint.Host, now.Add(tokenLifetime))
	if err != nil {
		return nil, err
	}

	body, err := encryptForSubscription(subscription, payload)
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequest(http.MethodPost, subscription.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("webpush: unable to create the request: %w", err)
	}

	request.Header.Set("Authorization", "vapid t="+token+", k="+publicKey)
	request.Header.Set("Content-Encoding", "aes128gcm")
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("TTL", strconv.Itoa(int(messageTTL.Seconds())))
	request.Header.Set("Urgency", "normal")

	return request, nil
}

func checkResponse(response *http.Response) error {
	switch {
	case response.StatusCode >= 200 && response.StatusCode < 300:
		return nil
	case response.StatusCode == http.StatusNotFound, response.StatusCode == http.StatusGone:
		return ErrSubscriptionGone
	}

	details, _ := io.ReadAll(io.LimitReader(response.Body, maxErrorBody))
	return fmt.Errorf("webpush: the push service answered %d: %s", response.StatusCode, strings.TrimSpace(string(details)))
}

func newToken(key *ecdsa.PrivateKey, audience string, expiresAt time.Time) (string, error) {
	claims, err := json.Marshal(map[string]any{
		"aud": audience,
		"exp": expiresAt.Unix(),
		"sub": contactURL,
	})
	if err != nil {
		return "", fmt.Errorf("webpush: unable to encode the token: %w", err)
	}

	encoding := base64.RawURLEncoding
	signingInput := encoding.EncodeToString([]byte(`{"typ":"JWT","alg":"ES256"}`)) + "." + encoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signingInput))

	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		return "", fmt.Errorf("webpush: unable to sign the token: %w", err)
	}

	signature := make([]byte, 64)
	r.FillBytes(signature[:32])
	s.FillBytes(signature[32:])

	return signingInput + "." + encoding.EncodeToString(signature), nil
}

func encryptForSubscription(subscription *Subscription, payload []byte) ([]byte, error) {
	clientPublicKey, err := decodeKey(subscription.P256DH)
	if err != nil {
		return nil, fmt.Errorf("webpush: invalid client key: %w", err)
	}

	authSecret, err := decodeKey(subscription.Auth)
	if err != nil {
		return nil, fmt.Errorf("webpush: invalid auth secret: %w", err)
	}

	serverKey, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("webpush: unable to generate a message key: %w", err)
	}

	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("webpush: unable to generate a salt: %w", err)
	}

	return encrypt(payload, clientPublicKey, authSecret, serverKey, salt)
}

func decodeKey(key string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(key, "="))
}

func encrypt(plaintext, clientPublicKey, authSecret []byte, serverKey *ecdh.PrivateKey, salt []byte) ([]byte, error) {
	clientKey, err := ecdh.P256().NewPublicKey(clientPublicKey)
	if err != nil {
		return nil, fmt.Errorf("webpush: invalid client key: %w", err)
	}

	sharedSecret, err := serverKey.ECDH(clientKey)
	if err != nil {
		return nil, fmt.Errorf("webpush: unable to derive the shared secret: %w", err)
	}

	serverPublicKey := serverKey.PublicKey().Bytes()
	keyInfo := slices.Concat([]byte("WebPush: info\x00"), clientPublicKey, serverPublicKey)

	inputKey, err := hkdf.Key(sha256.New, sharedSecret, authSecret, string(keyInfo), 32)
	if err != nil {
		return nil, fmt.Errorf("webpush: unable to derive the input key: %w", err)
	}

	contentKey, err := hkdf.Key(sha256.New, inputKey, salt, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, fmt.Errorf("webpush: unable to derive the content key: %w", err)
	}

	nonce, err := hkdf.Key(sha256.New, inputKey, salt, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, fmt.Errorf("webpush: unable to derive the nonce: %w", err)
	}

	block, err := aes.NewCipher(contentKey)
	if err != nil {
		return nil, fmt.Errorf("webpush: unable to create the cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("webpush: unable to create the cipher: %w", err)
	}

	header := binary.BigEndian.AppendUint32(slices.Clone(salt), recordSize)
	header = append(header, byte(len(serverPublicKey)))
	header = append(header, serverPublicKey...)

	record := append(slices.Clone(plaintext), 0x02)
	return aead.Seal(header, nonce, record, nil), nil
}
