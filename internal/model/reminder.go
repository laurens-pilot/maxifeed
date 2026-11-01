// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package model // import "miniflux.app/v2/internal/model"

// Reminder is the time of day a user wants to be nudged to open their feed.
type Reminder struct {
	Time     string
	Timezone string
}

// PushSubscription is a browser that agreed to receive notifications, in the
// shape returned by PushSubscription.toJSON() in the browser.
type PushSubscription struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256DH string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

// ReminderRequest turns a device's reminder on, or changes its time.
type ReminderRequest struct {
	Time         string            `json:"time"`
	Timezone     string            `json:"timezone"`
	Subscription *PushSubscription `json:"subscription"`
}
