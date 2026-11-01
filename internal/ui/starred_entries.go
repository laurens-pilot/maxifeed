// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui // import "miniflux.app/v2/internal/ui"

import (
	"net/http"

	"miniflux.app/v2/internal/http/request"
	"miniflux.app/v2/internal/http/response"
	"miniflux.app/v2/internal/ui/stream"
	"miniflux.app/v2/internal/ui/view"
)

func (h *handler) showStarredPage(w http.ResponseWriter, r *http.Request) {
	user, err := h.store.UserByID(request.UserID(r))
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	offset := request.QueryIntParam(r, "offset", 0)

	count, err := h.store.NewEntryQueryBuilder(user.ID).WithStarred(true).CountEntries()
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	entryIDs, err := h.store.SavedEntryIDs(user.ID, user.EntriesPerPage, offset)
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	cards, err := h.savedCards(user.ID, entryIDs)
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	destinations, err := h.sendDestinations(user.ID)
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	continuing, upNext := stream.SplitStarted(cards)

	view := view.New(h.tpl, r)
	view.Set("total", count)
	view.Set("continuing", continuing)
	view.Set("cards", upNext)
	view.Set("destinations", destinations)
	view.Set("pagination", getPagination(h.routePath("/starred"), count, offset, user.EntriesPerPage))
	view.Set("menu", "starred")
	view.Set("user", user)
	navMetadata, _ := h.store.GetNavMetadata(user.ID)
	view.Set("countErrorFeeds", navMetadata.CountErrorFeeds)

	response.HTML(w, r, view.Render("starred_entries"))
}
