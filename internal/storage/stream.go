// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage // import "miniflux.app/v2/internal/storage"

import (
	"fmt"
	"time"

	"github.com/lib/pq"

	"miniflux.app/v2/internal/model"
)

// StreamEntryIDs returns the next entries to show in the stream. Feeds take turns
// contributing their newest unread entry, so a busy feed can't bury a quiet one.
// Starred entries live in the saved list instead.
func (s *Storage) StreamEntryIDs(userID int64, limit int) ([]int64, error) {
	query := `
		WITH ranked AS (
			SELECT
				e.id,
				e.published_at,
				row_number() OVER (PARTITION BY e.feed_id ORDER BY e.published_at DESC, e.id DESC) AS feed_turn
			FROM
				entries e
				JOIN feeds f ON (f.id = e.feed_id)
				JOIN categories c ON (c.id = f.category_id)
			WHERE
				e.user_id=$1 AND
				e.status=$2 AND
				e.starred IS FALSE AND
				f.hide_globally IS FALSE AND
				c.hide_globally IS FALSE
		)
		SELECT
			id
		FROM
			ranked
		ORDER BY
			feed_turn ASC, published_at DESC, id DESC
		LIMIT $3
	`
	rows, err := s.db.Query(query, userID, model.EntryStatusUnread, limit)
	if err != nil {
		return nil, fmt.Errorf(`store: unable to fetch stream entries: %v`, err)
	}
	defer rows.Close()

	entryIDs := make([]int64, 0, limit)
	for rows.Next() {
		var entryID int64
		if err := rows.Scan(&entryID); err != nil {
			return nil, fmt.Errorf(`store: unable to fetch stream entries: %v`, err)
		}
		entryIDs = append(entryIDs, entryID)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(`store: unable to fetch stream entries: %v`, err)
	}

	return entryIDs, nil
}

// MarkStreamEntriesAsRead marks unread entries as read, credits them to the user's
// current reading day, and returns how many of them are visible in global views.
func (s *Storage) MarkStreamEntriesAsRead(userID int64, entryIDs []int64) (int, error) {
	query := `
		WITH updated AS (
			UPDATE entries
			SET
				status=$1,
				changed_at=now()
			WHERE
				user_id=$2 AND
				id=ANY($3) AND
				status=$4
			RETURNING feed_id
		), credited AS (
			INSERT INTO reading_days (user_id, day, read_count)
			SELECT
				u.id, (now() AT TIME ZONE u.timezone)::date, count(*)
			FROM
				updated, users u
			WHERE
				u.id=$2
			GROUP BY
				u.id, u.timezone
			ON CONFLICT (user_id, day) DO UPDATE SET read_count = reading_days.read_count + EXCLUDED.read_count
		)
		SELECT
			count(*)
		FROM
			updated u
			JOIN feeds f ON (f.id = u.feed_id)
			JOIN categories c ON (c.id = f.category_id)
		WHERE
			f.hide_globally IS FALSE AND
			c.hide_globally IS FALSE
	`
	var visible int
	err := s.db.QueryRow(query, model.EntryStatusRead, userID, pq.Array(entryIDs), model.EntryStatusUnread).Scan(&visible)
	if err != nil {
		return 0, fmt.Errorf(`store: unable to mark stream entries as read %v: %v`, entryIDs, err)
	}

	return visible, nil
}

// ClearStream marks everything still waiting in the stream as read, leaving starred
// entries and anything ingested after the given time untouched.
func (s *Storage) ClearStream(userID int64, before time.Time) error {
	query := `
		UPDATE
			entries
		SET
			status=$1,
			changed_at=now()
		FROM
			feeds
			JOIN categories ON (categories.id = feeds.category_id)
		WHERE
			entries.feed_id = feeds.id AND
			entries.user_id=$2 AND
			entries.status=$3 AND
			entries.starred IS FALSE AND
			entries.created_at <= $4 AND
			feeds.hide_globally IS FALSE AND
			categories.hide_globally IS FALSE
	`
	if _, err := s.db.Exec(query, model.EntryStatusRead, userID, model.EntryStatusUnread, before); err != nil {
		return fmt.Errorf(`store: unable to clear the stream: %v`, err)
	}

	return nil
}

// ReadingDays returns the user's reading days from newest to oldest.
func (s *Storage) ReadingDays(userID int64) ([]model.ReadingDay, error) {
	rows, err := s.db.Query(`SELECT day, read_count FROM reading_days WHERE user_id=$1 ORDER BY day DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf(`store: unable to fetch reading days: %v`, err)
	}
	defer rows.Close()

	var days []model.ReadingDay
	for rows.Next() {
		var day model.ReadingDay
		if err := rows.Scan(&day.Day, &day.ReadCount); err != nil {
			return nil, fmt.Errorf(`store: unable to fetch reading days: %v`, err)
		}
		days = append(days, day)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(`store: unable to fetch reading days: %v`, err)
	}

	return days, nil
}
