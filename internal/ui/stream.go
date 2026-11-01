// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui // import "miniflux.app/v2/internal/ui"

import (
	"cmp"
	json_parser "encoding/json"
	"errors"
	"net/http"
	"slices"
	"time"

	"miniflux.app/v2/internal/http/request"
	"miniflux.app/v2/internal/http/response"
	"miniflux.app/v2/internal/integration"
	"miniflux.app/v2/internal/model"
	"miniflux.app/v2/internal/timezone"
	"miniflux.app/v2/internal/ui/stream"
	"miniflux.app/v2/internal/ui/view"
	"miniflux.app/v2/internal/validator"
)

const streamBatchSize = 25

var errInvalidReadingPosition = errors.New("ui: invalid reading position")

func (h *handler) showStreamPage(w http.ResponseWriter, r *http.Request) {
	user, err := h.store.UserByID(request.UserID(r))
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	entries, err := h.nextStreamEntries(user.ID)
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	countWaiting, err := h.store.NewEntryQueryBuilder(user.ID).
		WithStatuses(model.EntryStatusUnread).
		WithStarred(false).
		WithGloballyVisible().
		CountEntries()
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	countSaved, err := h.store.NewEntryQueryBuilder(user.ID).
		WithStatuses(model.EntryStatusUnread).
		WithStarred(true).
		CountEntries()
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	readingDays, err := h.store.ReadingDays(user.ID)
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	pushSubscriptions, err := h.store.PushSubscriptions(user.ID)
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	continueCard, err := h.continueCard(user.ID)
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	destinations, err := h.sendDestinations(user.ID)
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	view := view.New(h.tpl, r)
	view.Set("cards", stream.NewCards(entries))
	view.Set("continueCard", continueCard)
	view.Set("destinations", destinations)
	view.Set("remaining", countWaiting-len(entries))
	view.Set("hasReminder", len(pushSubscriptions) > 0)
	view.Set("countSaved", countSaved)
	view.Set("stats", stream.NewStats(readingDays, timezone.Now(user.Timezone)))
	view.Set("loadedAt", time.Now().Unix())
	view.Set("menu", "stream")
	view.Set("user", user)
	navMetadata, _ := h.store.GetNavMetadata(user.ID)
	view.Set("countErrorFeeds", navMetadata.CountErrorFeeds)

	response.HTML(w, r, view.Render("stream"))
}

func (h *handler) nextStreamEntries(userID int64) (model.Entries, error) {
	entryIDs, err := h.store.StreamEntryIDs(userID, streamBatchSize)
	if err != nil {
		return nil, err
	}

	return h.entriesInOrder(userID, entryIDs)
}

func (h *handler) continueCard(userID int64) (*stream.Card, error) {
	entryID, err := h.store.ContinueEntryID(userID)
	if err != nil || entryID == 0 {
		return nil, err
	}

	cards, err := h.savedCards(userID, []int64{entryID})
	if err != nil || len(cards) == 0 {
		return nil, err
	}

	return cards[0], nil
}

func (h *handler) savedCards(userID int64, entryIDs []int64) ([]*stream.Card, error) {
	entries, err := h.entriesInOrder(userID, entryIDs)
	if err != nil {
		return nil, err
	}

	positions, err := h.store.ReadingPositions(userID, entryIDs)
	if err != nil {
		return nil, err
	}

	return stream.NewSavedCards(entries, positions), nil
}

func (h *handler) sendDestinations(userID int64) ([]integration.Destination, error) {
	userIntegrations, err := h.store.Integration(userID)
	if err != nil {
		return nil, err
	}

	return integration.Destinations(userIntegrations), nil
}

func (h *handler) entriesInOrder(userID int64, entryIDs []int64) (model.Entries, error) {
	if len(entryIDs) == 0 {
		return nil, nil
	}

	entries, err := h.store.NewEntryQueryBuilder(userID).
		WithEntryIDs(entryIDs...).
		WithEnclosures().
		GetEntries()
	if err != nil {
		return nil, err
	}

	slices.SortFunc(entries, func(a, b *model.Entry) int {
		return cmp.Compare(slices.Index(entryIDs, a.ID), slices.Index(entryIDs, b.ID))
	})

	return entries, nil
}

func (h *handler) showStreamEntry(w http.ResponseWriter, r *http.Request) {
	user, err := h.store.UserByID(request.UserID(r))
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	entry, err := h.store.NewEntryQueryBuilder(user.ID).
		WithEntryIDs(request.RouteInt64Param(r, "entryID")).
		WithEnclosures().
		GetEntry()
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	if entry == nil {
		response.HTMLNotFound(w, r)
		return
	}

	navMetadata, err := h.store.GetNavMetadata(user.ID)
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	destinations, err := h.sendDestinations(user.ID)
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	positions, err := h.store.ReadingPositions(user.ID, []int64{entry.ID})
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	view := view.New(h.tpl, r)
	view.Set("entry", entry)
	view.Set("user", user)
	view.Set("position", positions[entry.ID])
	view.Set("hasSaveEntry", navMetadata.HasSaveEntry && len(destinations) == 0)

	response.HTMLFragment(w, r, view.RenderFragment("stream", "reader_content"))
}

func (h *handler) saveReadingPosition(w http.ResponseWriter, r *http.Request) {
	var position model.ReadingPosition
	if err := json_parser.NewDecoder(r.Body).Decode(&position); err != nil {
		response.JSONBadRequest(w, r, err)
		return
	}

	if !position.IsValid() {
		response.JSONBadRequest(w, r, errInvalidReadingPosition)
		return
	}

	if err := h.store.SaveReadingPosition(request.UserID(r), request.RouteInt64Param(r, "entryID"), &position); err != nil {
		response.JSONServerError(w, r, err)
		return
	}

	response.JSON(w, r, "OK")
}

func (h *handler) finishSavedEntry(w http.ResponseWriter, r *http.Request) {
	if err := h.store.FinishSavedEntry(request.UserID(r), request.RouteInt64Param(r, "entryID")); err != nil {
		response.JSONServerError(w, r, err)
		return
	}

	response.JSON(w, r, "OK")
}

func (h *handler) markStreamEntriesAsRead(w http.ResponseWriter, r *http.Request) {
	var entriesStatusUpdateRequest model.EntriesStatusUpdateRequest
	if err := json_parser.NewDecoder(r.Body).Decode(&entriesStatusUpdateRequest); err != nil {
		response.JSONBadRequest(w, r, err)
		return
	}

	entriesStatusUpdateRequest.Status = model.EntryStatusRead
	if err := validator.ValidateEntriesStatusUpdateRequest(&entriesStatusUpdateRequest); err != nil {
		response.JSONBadRequest(w, r, err)
		return
	}

	count, err := h.store.MarkStreamEntriesAsRead(request.UserID(r), entriesStatusUpdateRequest.EntryIDs)
	if err != nil {
		response.JSONServerError(w, r, err)
		return
	}

	response.JSON(w, r, count)
}

func (h *handler) clearStream(w http.ResponseWriter, r *http.Request) {
	before := time.Unix(request.QueryInt64Param(r, "before", 0), 0)
	if err := h.store.ClearStream(request.UserID(r), before); err != nil {
		response.JSONServerError(w, r, err)
		return
	}

	response.JSON(w, r, "OK")
}
