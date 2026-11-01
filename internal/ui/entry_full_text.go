// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui // import "miniflux.app/v2/internal/ui"

import (
	"log/slog"

	"miniflux.app/v2/internal/reader/processor"
	"miniflux.app/v2/internal/ui/stream"
)

func (h *handler) fetchFullTextOfSavedEntry(userID, entryID int64) {
	if err := h.replaceSummaryWithFullText(userID, entryID); err != nil {
		slog.Warn("Unable to fetch the full text of a saved entry",
			slog.Int64("user_id", userID),
			slog.Int64("entry_id", entryID),
			slog.Any("error", err),
		)
	}
}

func (h *handler) replaceSummaryWithFullText(userID, entryID int64) error {
	entry, err := h.store.NewEntryQueryBuilder(userID).
		WithEntryIDs(entryID).
		WithEnclosures().
		GetEntry()
	if err != nil || entry == nil || !entry.Starred || !stream.LooksLikeSummary(entry) {
		return err
	}

	feed, err := h.store.NewFeedQueryBuilder(userID).WithFeedID(entry.FeedID).GetFeed()
	if err != nil || feed == nil || feed.Crawler {
		return err
	}

	user, err := h.store.UserByID(userID)
	if err != nil || user == nil {
		return err
	}

	summary := entry.Content
	if err := processor.ProcessEntryWebPage(feed, entry, user); err != nil {
		return err
	}

	if !stream.IsFullText(entry.Content, summary) {
		return nil
	}

	return h.store.UpdateEntryTitleAndContent(entry)
}
