// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui // import "miniflux.app/v2/internal/ui"

import (
	json_parser "encoding/json"
	"errors"
	"net/http"
	"time"

	"miniflux.app/v2/internal/http/request"
	"miniflux.app/v2/internal/http/response"
	"miniflux.app/v2/internal/model"
	"miniflux.app/v2/internal/reminder"
	"miniflux.app/v2/internal/ui/view"
	"miniflux.app/v2/internal/urllib"
	"miniflux.app/v2/internal/webpush"
)

const defaultReminderTime = "08:00"

func (h *handler) showReminderPage(w http.ResponseWriter, r *http.Request) {
	user, err := h.store.UserByID(request.UserID(r))
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	savedReminder, err := h.store.Reminder(user.ID)
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	reminderTime := defaultReminderTime
	if savedReminder != nil {
		reminderTime = savedReminder.Time
	}

	privateKey, err := h.store.VAPIDPrivateKey()
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	publicKey, err := webpush.PublicKey(privateKey)
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	view := view.New(h.tpl, r)
	view.Set("reminderTime", reminderTime)
	view.Set("publicKey", publicKey)
	view.Set("menu", "settings")
	view.Set("user", user)
	navMetadata, _ := h.store.GetNavMetadata(user.ID)
	view.Set("countErrorFeeds", navMetadata.CountErrorFeeds)

	response.HTML(w, r, view.Render("reminder"))
}

func (h *handler) saveReminder(w http.ResponseWriter, r *http.Request) {
	user, err := h.store.UserByID(request.UserID(r))
	if err != nil {
		response.JSONServerError(w, r, err)
		return
	}

	var reminderRequest model.ReminderRequest
	if err := json_parser.NewDecoder(r.Body).Decode(&reminderRequest); err != nil {
		response.JSONBadRequest(w, r, err)
		return
	}

	if _, err := time.Parse("15:04", reminderRequest.Time); err != nil {
		response.JSONBadRequest(w, r, err)
		return
	}

	if err := validatePushSubscription(reminderRequest.Subscription); err != nil {
		response.JSONBadRequest(w, r, err)
		return
	}

	reminderTimezone := reminderRequest.Timezone
	known, err := h.store.IsKnownTimezone(reminderTimezone)
	if err != nil {
		response.JSONServerError(w, r, err)
		return
	}

	if !known {
		reminderTimezone = user.Timezone
	}

	if err := h.store.SavePushSubscription(user.ID, reminderRequest.Subscription); err != nil {
		response.JSONServerError(w, r, err)
		return
	}

	if err := h.store.SaveReminder(user.ID, &model.Reminder{Time: reminderRequest.Time, Timezone: reminderTimezone}); err != nil {
		response.JSONServerError(w, r, err)
		return
	}

	response.JSON(w, r, "OK")
}

func (h *handler) syncReminderDevice(w http.ResponseWriter, r *http.Request) {
	var reminderRequest model.ReminderRequest
	if err := json_parser.NewDecoder(r.Body).Decode(&reminderRequest); err != nil {
		response.JSONBadRequest(w, r, err)
		return
	}

	if err := validatePushSubscription(reminderRequest.Subscription); err != nil {
		response.JSONBadRequest(w, r, err)
		return
	}

	userID := request.UserID(r)
	if err := h.store.SavePushSubscription(userID, reminderRequest.Subscription); err != nil {
		response.JSONServerError(w, r, err)
		return
	}

	if err := h.store.FollowDeviceTimezone(userID, reminderRequest.Timezone); err != nil {
		response.JSONServerError(w, r, err)
		return
	}

	response.JSON(w, r, "OK")
}

func (h *handler) removeReminderDevice(w http.ResponseWriter, r *http.Request) {
	var subscription model.PushSubscription
	if err := json_parser.NewDecoder(r.Body).Decode(&subscription); err != nil {
		response.JSONBadRequest(w, r, err)
		return
	}

	if err := h.store.RemovePushSubscription(request.UserID(r), subscription.Endpoint); err != nil {
		response.JSONServerError(w, r, err)
		return
	}

	response.JSON(w, r, "OK")
}

func (h *handler) sendTestReminder(w http.ResponseWriter, r *http.Request) {
	user, err := h.store.UserByID(request.UserID(r))
	if err != nil {
		response.JSONServerError(w, r, err)
		return
	}

	if err := reminder.SendTest(h.store, user); err != nil {
		response.JSONServerError(w, r, err)
		return
	}

	response.JSON(w, r, "OK")
}

func validatePushSubscription(subscription *model.PushSubscription) error {
	switch {
	case subscription == nil:
		return errors.New("the subscription is missing")
	case !urllib.IsHTTPS(subscription.Endpoint):
		return errors.New("the push endpoint must use HTTPS")
	case subscription.Keys.P256DH == "" || subscription.Keys.Auth == "":
		return errors.New("the subscription keys are missing")
	}

	return nil
}
