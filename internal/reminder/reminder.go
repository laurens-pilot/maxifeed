// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package reminder sends the daily nudge to open the feed.
package reminder // import "miniflux.app/v2/internal/reminder"

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"miniflux.app/v2/internal/locale"
	"miniflux.app/v2/internal/model"
	"miniflux.app/v2/internal/storage"
	"miniflux.app/v2/internal/timezone"
	"miniflux.app/v2/internal/ui/stream"
	"miniflux.app/v2/internal/webpush"
)

const maxHeadlineLength = 120

// ErrNoDevices is returned when the user has no device left to notify.
var ErrNoDevices = errors.New("reminder: no device is subscribed")

type message struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// SendDue sends the reminders whose time has arrived.
func SendDue(store *storage.Storage) {
	userIDs, err := store.ClaimDueReminders()
	if err != nil {
		slog.Error("Unable to claim due reminders", slog.Any("error", err))
		return
	}

	for _, userID := range userIDs {
		if err := sendDaily(store, userID); err != nil {
			slog.Error("Unable to send the daily reminder", slog.Int64("user_id", userID), slog.Any("error", err))
		}
	}
}

// SendTest sends a sample notification to every device of the user.
func SendTest(store *storage.Storage, user *model.User) error {
	printer := locale.NewPrinter(user.Language)
	return send(store, user.ID, &message{
		Title: printer.Print("reminder.test.title"),
		Body:  printer.Print("reminder.test.body"),
	})
}

func sendDaily(store *storage.Storage, userID int64) error {
	user, err := store.UserByID(userID)
	if err != nil {
		return err
	}

	if user == nil {
		return nil
	}

	waiting, err := store.NewEntryQueryBuilder(userID).
		WithStatuses(model.EntryStatusUnread).
		WithStarred(false).
		WithGloballyVisible().
		CountEntries()
	if err != nil {
		return err
	}

	continuing, err := continuingTitle(store, userID)
	if err != nil {
		return err
	}

	if waiting == 0 && continuing == "" {
		return nil
	}

	headline, err := nextHeadline(store, userID)
	if err != nil {
		return err
	}

	readingDays, err := store.ReadingDays(userID)
	if err != nil {
		return err
	}

	stats := stream.NewStats(readingDays, timezone.Now(user.Timezone))
	return send(store, userID, compose(locale.NewPrinter(user.Language), stats, waiting, headline, continuing))
}

func nextHeadline(store *storage.Storage, userID int64) (string, error) {
	entryIDs, err := store.StreamEntryIDs(userID, 1)
	if err != nil || len(entryIDs) == 0 {
		return "", err
	}

	return entryTitle(store, userID, entryIDs[0])
}

func continuingTitle(store *storage.Storage, userID int64) (string, error) {
	entryID, err := store.ContinueEntryID(userID)
	if err != nil || entryID == 0 {
		return "", err
	}

	positions, err := store.ReadingPositions(userID, []int64{entryID})
	if err != nil || positions[entryID] == nil {
		return "", err
	}

	return entryTitle(store, userID, entryID)
}

func entryTitle(store *storage.Storage, userID, entryID int64) (string, error) {
	entry, err := store.NewEntryQueryBuilder(userID).WithEntryIDs(entryID).GetEntry()
	if err != nil || entry == nil {
		return "", err
	}

	return entry.Title, nil
}

func compose(printer *locale.Printer, stats stream.Stats, waiting int, headline, continuing string) *message {
	title := printer.Print("reminder.title")
	if stats.Streak > 0 && stats.ReadToday == 0 {
		title = printer.Printf("reminder.title.streak", stats.Streak)
	}

	return &message{
		Title: title,
		Body:  composeBody(printer, waiting, headline, continuing),
	}
}

func composeBody(printer *locale.Printer, waiting int, headline, continuing string) string {
	switch {
	case continuing == "":
		return printer.Printf("reminder.body", waiting, truncate(headline))
	case waiting == 0:
		return printer.Printf("reminder.body.continue", truncate(continuing))
	default:
		return printer.Printf("reminder.body.waiting_continue", waiting, truncate(continuing))
	}
}

func truncate(headline string) string {
	runes := []rune(headline)
	if len(runes) <= maxHeadlineLength {
		return headline
	}

	return string(runes[:maxHeadlineLength]) + "…"
}

func send(store *storage.Storage, userID int64, content *message) error {
	privateKey, err := store.VAPIDPrivateKey()
	if err != nil {
		return err
	}

	subscriptions, err := store.PushSubscriptions(userID)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(content)
	if err != nil {
		return fmt.Errorf("reminder: unable to encode the message: %w", err)
	}

	delivered := 0
	var failures []error
	for _, subscription := range subscriptions {
		err := webpush.Send(privateKey, &webpush.Subscription{
			Endpoint: subscription.Endpoint,
			P256DH:   subscription.Keys.P256DH,
			Auth:     subscription.Keys.Auth,
		}, payload)

		switch {
		case err == nil:
			delivered++
		case errors.Is(err, webpush.ErrSubscriptionGone):
			if err := store.RemovePushSubscription(userID, subscription.Endpoint); err != nil {
				failures = append(failures, err)
			}
		default:
			failures = append(failures, err)
		}
	}

	if len(failures) > 0 {
		return errors.Join(failures...)
	}

	if delivered == 0 {
		return ErrNoDevices
	}

	return nil
}
