// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage // import "miniflux.app/v2/internal/storage"

import (
	"database/sql"
	"errors"
	"fmt"

	"miniflux.app/v2/internal/model"
)

// VAPIDPrivateKey returns the key that identifies this server to push services.
func (s *Storage) VAPIDPrivateKey() ([]byte, error) {
	var privateKey []byte
	if err := s.db.QueryRow(`SELECT private_key FROM vapid_keys`).Scan(&privateKey); err != nil {
		return nil, fmt.Errorf(`store: unable to fetch the VAPID key: %v`, err)
	}

	return privateKey, nil
}

// Reminder returns the user's reminder, or nil if they never set one.
func (s *Storage) Reminder(userID int64) (*model.Reminder, error) {
	query := `SELECT to_char(remind_at, 'HH24:MI'), timezone FROM reminders WHERE user_id=$1`

	var reminder model.Reminder
	err := s.db.QueryRow(query, userID).Scan(&reminder.Time, &reminder.Timezone)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf(`store: unable to fetch the reminder of user #%d: %v`, userID, err)
	}

	return &reminder, nil
}

// SaveReminder sets the time of the user's daily reminder. A time that already
// passed today takes effect tomorrow, so saving never fires a reminder right away.
func (s *Storage) SaveReminder(userID int64, reminder *model.Reminder) error {
	query := `
		WITH local AS (
			SELECT
				(now() AT TIME ZONE $3::text)::date AS today,
				(now() AT TIME ZONE $3::text)::time AS time_of_day
		)
		INSERT INTO reminders (user_id, remind_at, timezone, last_sent_on)
		SELECT
			$1, $2::time, $3, CASE WHEN time_of_day >= $2::time THEN today ELSE today - 1 END
		FROM
			local
		ON CONFLICT (user_id) DO UPDATE SET
			remind_at=EXCLUDED.remind_at,
			timezone=EXCLUDED.timezone,
			last_sent_on=EXCLUDED.last_sent_on
	`
	if _, err := s.db.Exec(query, userID, reminder.Time, reminder.Timezone); err != nil {
		return fmt.Errorf(`store: unable to save the reminder of user #%d: %v`, userID, err)
	}

	return nil
}

// IsKnownTimezone reports whether Postgres can resolve the timezone. Browsers
// often report legacy names that are valid here but missing from the settings list.
func (s *Storage) IsKnownTimezone(timezone string) (bool, error) {
	var known bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=$1)`, timezone).Scan(&known)
	if err != nil {
		return false, fmt.Errorf(`store: unable to check the timezone %q: %v`, timezone, err)
	}

	return known, nil
}

// FollowDeviceTimezone keeps the reminder on the device's local time when the user travels.
func (s *Storage) FollowDeviceTimezone(userID int64, timezone string) error {
	query := `
		UPDATE
			reminders
		SET
			timezone=$2
		WHERE
			user_id=$1 AND
			timezone<>$2 AND
			EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=$2)
	`
	if _, err := s.db.Exec(query, userID, timezone); err != nil {
		return fmt.Errorf(`store: unable to update the reminder timezone of user #%d: %v`, userID, err)
	}

	return nil
}

// ClaimDueReminders returns the users whose reminder time has arrived and records
// it as sent for their local day, so that each reminder goes out at most once.
func (s *Storage) ClaimDueReminders() ([]int64, error) {
	query := `
		UPDATE
			reminders
		SET
			last_sent_on=(now() AT TIME ZONE timezone)::date
		WHERE
			(now() AT TIME ZONE timezone)::time >= remind_at AND
			(now() AT TIME ZONE timezone)::date > last_sent_on
		RETURNING
			user_id
	`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf(`store: unable to claim due reminders: %v`, err)
	}
	defer rows.Close()

	var userIDs []int64
	for rows.Next() {
		var userID int64
		if err := rows.Scan(&userID); err != nil {
			return nil, fmt.Errorf(`store: unable to claim due reminders: %v`, err)
		}
		userIDs = append(userIDs, userID)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(`store: unable to claim due reminders: %v`, err)
	}

	return userIDs, nil
}

// SavePushSubscription registers a device for the user.
func (s *Storage) SavePushSubscription(userID int64, subscription *model.PushSubscription) error {
	query := `
		INSERT INTO push_subscriptions (endpoint, user_id, p256dh, auth)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (endpoint) DO UPDATE SET
			user_id=EXCLUDED.user_id,
			p256dh=EXCLUDED.p256dh,
			auth=EXCLUDED.auth
	`
	_, err := s.db.Exec(query, subscription.Endpoint, userID, subscription.Keys.P256DH, subscription.Keys.Auth)
	if err != nil {
		return fmt.Errorf(`store: unable to save a push subscription of user #%d: %v`, userID, err)
	}

	return nil
}

// RemovePushSubscription forgets a device of the user.
func (s *Storage) RemovePushSubscription(userID int64, endpoint string) error {
	_, err := s.db.Exec(`DELETE FROM push_subscriptions WHERE user_id=$1 AND endpoint=$2`, userID, endpoint)
	if err != nil {
		return fmt.Errorf(`store: unable to remove a push subscription of user #%d: %v`, userID, err)
	}

	return nil
}

// PushSubscriptions returns the devices of the user.
func (s *Storage) PushSubscriptions(userID int64) ([]*model.PushSubscription, error) {
	rows, err := s.db.Query(`SELECT endpoint, p256dh, auth FROM push_subscriptions WHERE user_id=$1`, userID)
	if err != nil {
		return nil, fmt.Errorf(`store: unable to fetch the push subscriptions of user #%d: %v`, userID, err)
	}
	defer rows.Close()

	var subscriptions []*model.PushSubscription
	for rows.Next() {
		var subscription model.PushSubscription
		if err := rows.Scan(&subscription.Endpoint, &subscription.Keys.P256DH, &subscription.Keys.Auth); err != nil {
			return nil, fmt.Errorf(`store: unable to fetch the push subscriptions of user #%d: %v`, userID, err)
		}
		subscriptions = append(subscriptions, &subscription)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(`store: unable to fetch the push subscriptions of user #%d: %v`, userID, err)
	}

	return subscriptions, nil
}
