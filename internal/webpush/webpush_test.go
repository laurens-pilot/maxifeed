// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

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
	"io"
	"math/big"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
)

func mustDecode(t *testing.T, value string) []byte {
	t.Helper()

	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		t.Fatalf("unable to decode %q: %v", value, err)
	}

	return decoded
}

func decrypt(t *testing.T, body []byte, clientKey *ecdh.PrivateKey, authSecret []byte) []byte {
	t.Helper()

	salt := body[:16]
	if size := binary.BigEndian.Uint32(body[16:20]); size != recordSize {
		t.Fatalf("unexpected record size %d", size)
	}

	keyLength := int(body[20])
	serverPublicKey := body[21 : 21+keyLength]
	serverKey, err := ecdh.P256().NewPublicKey(serverPublicKey)
	if err != nil {
		t.Fatalf("invalid server key: %v", err)
	}

	sharedSecret, err := clientKey.ECDH(serverKey)
	if err != nil {
		t.Fatalf("unable to derive the shared secret: %v", err)
	}

	keyInfo := slices.Concat([]byte("WebPush: info\x00"), clientKey.PublicKey().Bytes(), serverPublicKey)
	inputKey, err := hkdf.Key(sha256.New, sharedSecret, authSecret, string(keyInfo), 32)
	if err != nil {
		t.Fatal(err)
	}

	contentKey, err := hkdf.Key(sha256.New, inputKey, salt, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		t.Fatal(err)
	}

	nonce, err := hkdf.Key(sha256.New, inputKey, salt, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		t.Fatal(err)
	}

	block, err := aes.NewCipher(contentKey)
	if err != nil {
		t.Fatal(err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}

	record, err := aead.Open(nil, nonce, body[21+keyLength:], nil)
	if err != nil {
		t.Fatalf("unable to decrypt the record: %v", err)
	}

	if record[len(record)-1] != 0x02 {
		t.Fatalf("missing the last record delimiter: %x", record)
	}

	return record[:len(record)-1]
}

func TestEncryptMatchesTheRFC8291Example(t *testing.T) {
	serverKey, err := ecdh.P256().NewPrivateKey(mustDecode(t, "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"))
	if err != nil {
		t.Fatal(err)
	}

	body, err := encrypt(
		[]byte("When I grow up, I want to be a watermelon"),
		mustDecode(t, "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4"),
		mustDecode(t, "BTBZMqHH6r4Tts7J_aSIgg"),
		serverKey,
		mustDecode(t, "DGv6ra1nlYgDCS1FRnbzlw"),
	)
	if err != nil {
		t.Fatal(err)
	}

	expected := "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPTpK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN"
	if result := base64.RawURLEncoding.EncodeToString(body); result != expected {
		t.Errorf("unexpected ciphertext:\ngot  %s\nwant %s", result, expected)
	}
}

func TestNewRequestProducesAMessageTheBrowserCanRead(t *testing.T) {
	privateKey, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	clientKey, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	authSecret := make([]byte, 16)
	if _, err := rand.Read(authSecret); err != nil {
		t.Fatal(err)
	}

	subscription := &Subscription{
		Endpoint: "https://push.example.org/send/abc",
		P256DH:   base64.RawURLEncoding.EncodeToString(clientKey.PublicKey().Bytes()),
		Auth:     base64.RawURLEncoding.EncodeToString(authSecret),
	}
	payload := []byte(`{"title":"Your feed is ready"}`)

	request, err := newRequest(privateKey, subscription, payload, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	if request.Header.Get("Content-Encoding") != "aes128gcm" {
		t.Errorf("unexpected content encoding %q", request.Header.Get("Content-Encoding"))
	}

	if request.Header.Get("TTL") != "21600" {
		t.Errorf("unexpected TTL %q", request.Header.Get("TTL"))
	}

	body, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatal(err)
	}

	if result := decrypt(t, body, clientKey, authSecret); !bytes.Equal(result, payload) {
		t.Errorf("unexpected payload %q", result)
	}
}

func TestVAPIDHeaderCanBeVerifiedByThePushService(t *testing.T) {
	privateKey, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	publicKey, err := PublicKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}

	clientKey, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Unix(1_800_000_000, 0)
	request, err := newRequest(privateKey, &Subscription{
		Endpoint: "https://web.push.apple.com/some/device?x=1",
		P256DH:   base64.RawURLEncoding.EncodeToString(clientKey.PublicKey().Bytes()),
		Auth:     "BTBZMqHH6r4Tts7J_aSIgg",
	}, []byte("{}"), now)
	if err != nil {
		t.Fatal(err)
	}

	header, found := strings.CutPrefix(request.Header.Get("Authorization"), "vapid t=")
	if !found {
		t.Fatalf("unexpected header %q", request.Header.Get("Authorization"))
	}

	token, key, found := strings.Cut(header, ", k=")
	if !found || key != publicKey {
		t.Fatalf("unexpected key in %q", header)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("unexpected token %q", token)
	}

	var claims struct {
		Audience  string `json:"aud"`
		ExpiresAt int64  `json:"exp"`
		Subject   string `json:"sub"`
	}
	if err := json.Unmarshal(mustDecode(t, parts[1]), &claims); err != nil {
		t.Fatal(err)
	}

	if claims.Audience != "https://web.push.apple.com" {
		t.Errorf("unexpected audience %q", claims.Audience)
	}

	if claims.ExpiresAt != now.Add(tokenLifetime).Unix() {
		t.Errorf("unexpected expiry %d", claims.ExpiresAt)
	}

	if !strings.HasPrefix(claims.Subject, "https://") {
		t.Errorf("Apple rejects any subject that is not a mailto or https address, got %q", claims.Subject)
	}

	verifyingKey, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), mustDecode(t, publicKey))
	if err != nil {
		t.Fatal(err)
	}

	signature := mustDecode(t, parts[2])
	if len(signature) != 64 {
		t.Fatalf("unexpected signature length %d", len(signature))
	}

	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:])
	if !ecdsa.Verify(verifyingKey, digest[:], r, s) {
		t.Error("the signature does not verify against the advertised public key")
	}
}

func TestCheckResponse(t *testing.T) {
	scenarios := []struct {
		status   int
		expected error
		failed   bool
	}{
		{http.StatusCreated, nil, false},
		{http.StatusNotFound, ErrSubscriptionGone, true},
		{http.StatusGone, ErrSubscriptionGone, true},
		{http.StatusForbidden, nil, true},
		{http.StatusTooManyRequests, nil, true},
	}

	for _, scenario := range scenarios {
		err := checkResponse(&http.Response{StatusCode: scenario.status, Body: io.NopCloser(strings.NewReader("details"))})
		if (err != nil) != scenario.failed {
			t.Errorf("status %d: unexpected result %v", scenario.status, err)
		}

		if scenario.expected != nil && !errors.Is(err, scenario.expected) {
			t.Errorf("status %d: expected %v, got %v", scenario.status, scenario.expected, err)
		}
	}
}
