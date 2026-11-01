// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage // import "miniflux.app/v2/internal/storage"

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"

	"miniflux.app/v2/internal/model"
)

func (s *Storage) SavedEntryIDs(userID int64, limit, offset int) ([]int64, error) {
	query := `
		SELECT
			e.id
		FROM
			entries e
			LEFT JOIN reading_positions p ON (p.entry_id = e.id AND p.block > 0)
		WHERE
			e.user_id=$1 AND
			e.starred IS TRUE
		ORDER BY
			p.updated_at DESC NULLS LAST, e.changed_at DESC, e.id DESC
		LIMIT $2
		OFFSET $3
	`
	rows, err := s.db.Query(query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf(`store: unable to fetch saved entries: %v`, err)
	}
	defer rows.Close()

	entryIDs := make([]int64, 0, limit)
	for rows.Next() {
		var entryID int64
		if err := rows.Scan(&entryID); err != nil {
			return nil, fmt.Errorf(`store: unable to fetch saved entries: %v`, err)
		}
		entryIDs = append(entryIDs, entryID)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(`store: unable to fetch saved entries: %v`, err)
	}

	return entryIDs, nil
}

func (s *Storage) ContinueEntryID(userID int64) (int64, error) {
	query := `
		SELECT
			e.id
		FROM
			entries e
			LEFT JOIN reading_positions p ON (p.entry_id = e.id AND p.block > 0)
		WHERE
			e.user_id=$1 AND
			e.starred IS TRUE AND
			(p.entry_id IS NOT NULL OR e.status=$2)
		ORDER BY
			p.updated_at DESC NULLS LAST, e.changed_at DESC, e.id DESC
		LIMIT 1
	`
	var entryID int64
	err := s.db.QueryRow(query, userID, model.EntryStatusUnread).Scan(&entryID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return 0, nil
	case err != nil:
		return 0, fmt.Errorf(`store: unable to fetch the entry to continue: %v`, err)
	}

	return entryID, nil
}

func (s *Storage) ReadingPositions(userID int64, entryIDs []int64) (map[int64]*model.ReadingPosition, error) {
	query := `SELECT entry_id, block, block_count FROM reading_positions WHERE user_id=$1 AND entry_id=ANY($2) AND block > 0`
	rows, err := s.db.Query(query, userID, pq.Array(entryIDs))
	if err != nil {
		return nil, fmt.Errorf(`store: unable to fetch reading positions: %v`, err)
	}
	defer rows.Close()

	positions := make(map[int64]*model.ReadingPosition, len(entryIDs))
	for rows.Next() {
		var entryID int64
		var position model.ReadingPosition
		if err := rows.Scan(&entryID, &position.Block, &position.Blocks); err != nil {
			return nil, fmt.Errorf(`store: unable to fetch reading positions: %v`, err)
		}
		positions[entryID] = &position
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(`store: unable to fetch reading positions: %v`, err)
	}

	return positions, nil
}

func (s *Storage) SaveReadingPosition(userID, entryID int64, position *model.ReadingPosition) error {
	query := `
		WITH saved AS (
			INSERT INTO reading_positions (entry_id, user_id, block, block_count)
			SELECT
				id, user_id, $3, $4
			FROM
				entries
			WHERE
				id=$1 AND
				user_id=$2 AND
				starred IS TRUE
			ON CONFLICT (entry_id) DO UPDATE SET
				block=EXCLUDED.block,
				block_count=EXCLUDED.block_count,
				updated_at=now()
			RETURNING user_id
		)
		INSERT INTO reading_days (user_id, day, read_count)
		SELECT
			u.id, (now() AT TIME ZONE u.timezone)::date, 1
		FROM
			saved
			JOIN users u ON (u.id = saved.user_id)
		ON CONFLICT (user_id, day) DO NOTHING
	`
	if _, err := s.db.Exec(query, entryID, userID, position.Block, position.Blocks); err != nil {
		return fmt.Errorf(`store: unable to save the reading position of entry #%d: %v`, entryID, err)
	}

	return nil
}

func (s *Storage) FinishSavedEntry(userID, entryID int64) error {
	query := `
		WITH forgotten AS (
			DELETE FROM reading_positions WHERE user_id=$1 AND entry_id=$2
		)
		UPDATE entries SET starred=false, changed_at=now() WHERE user_id=$1 AND id=$2
	`
	if _, err := s.db.Exec(query, userID, entryID); err != nil {
		return fmt.Errorf(`store: unable to finish entry #%d: %v`, entryID, err)
	}

	return nil
}
