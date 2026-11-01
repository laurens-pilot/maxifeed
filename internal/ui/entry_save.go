// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui // import "miniflux.app/v2/internal/ui"

import (
	"errors"
	"net/http"

	"miniflux.app/v2/internal/http/request"
	"miniflux.app/v2/internal/http/response"
	"miniflux.app/v2/internal/integration"
)

func (h *handler) sendEntry(w http.ResponseWriter, r *http.Request) {
	entry, err := h.store.NewEntryQueryBuilder(request.UserID(r)).
		WithEntryIDs(request.RouteInt64Param(r, "entryID")).
		GetEntry()
	if err != nil {
		response.JSONServerError(w, r, err)
		return
	}

	if entry == nil {
		response.JSONNotFound(w, r)
		return
	}

	userIntegrations, err := h.store.Integration(request.UserID(r))
	if err != nil {
		response.JSONServerError(w, r, err)
		return
	}

	err = integration.SendEntryTo(entry, userIntegrations, request.RouteStringParam(r, "destination"))
	switch {
	case errors.Is(err, integration.ErrDestinationUnavailable):
		response.JSONBadRequest(w, r, err)
	case err != nil:
		response.JSONServerError(w, r, err)
	default:
		response.JSONCreated(w, r, map[string]string{"message": "saved"})
	}
}

func (h *handler) saveEntry(w http.ResponseWriter, r *http.Request) {
	entryID := request.RouteInt64Param(r, "entryID")

	entry, err := h.store.NewEntryQueryBuilder(request.UserID(r)).
		WithEntryIDs(entryID).
		GetEntry()
	if err != nil {
		response.JSONServerError(w, r, err)
		return
	}

	if entry == nil {
		response.JSONNotFound(w, r)
		return
	}

	userIntegrations, err := h.store.Integration(request.UserID(r))
	if err != nil {
		response.JSONServerError(w, r, err)
		return
	}

	go integration.SendEntry(entry, userIntegrations)

	response.JSONCreated(w, r, map[string]string{"message": "saved"})
}
