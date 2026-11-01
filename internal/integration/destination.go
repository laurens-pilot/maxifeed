// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package integration // import "miniflux.app/v2/internal/integration"

import (
	"errors"

	"miniflux.app/v2/internal/integration/raindrop"
	"miniflux.app/v2/internal/integration/readeck"
	"miniflux.app/v2/internal/model"
)

const (
	readeckDestination  = "readeck"
	raindropDestination = "raindrop"
)

var ErrDestinationUnavailable = errors.New("integration: this destination is not enabled")

type Destination struct {
	ID   string
	Name string
}

func Destinations(userIntegrations *model.Integration) []Destination {
	var destinations []Destination

	if userIntegrations.ReadeckEnabled {
		destinations = append(destinations, Destination{ID: readeckDestination, Name: "Readeck"})
	}

	if userIntegrations.RaindropEnabled {
		destinations = append(destinations, Destination{ID: raindropDestination, Name: "Raindrop"})
	}

	return destinations
}

func SendEntryTo(entry *model.Entry, userIntegrations *model.Integration, destinationID string) error {
	switch {
	case destinationID == readeckDestination && userIntegrations.ReadeckEnabled:
		client := readeck.NewClient(
			userIntegrations.ReadeckURL,
			userIntegrations.ReadeckAPIKey,
			userIntegrations.ReadeckLabels,
			userIntegrations.ReadeckOnlyURL,
		)
		return client.CreateBookmark(entry.URL, entry.Title, entry.Content)
	case destinationID == raindropDestination && userIntegrations.RaindropEnabled:
		client := raindrop.NewClient(userIntegrations.RaindropToken, userIntegrations.RaindropCollectionID, userIntegrations.RaindropTags)
		return client.CreateRaindrop(entry.URL, entry.Title)
	}

	return ErrDestinationUnavailable
}
