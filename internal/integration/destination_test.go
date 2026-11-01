// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package integration // import "miniflux.app/v2/internal/integration"

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"miniflux.app/v2/internal/config"
	"miniflux.app/v2/internal/model"
)

func TestDestinationsListsOnlyEnabledServices(t *testing.T) {
	scenarios := []struct {
		name     string
		settings model.Integration
		expected []string
	}{
		{"none", model.Integration{}, nil},
		{"readeck", model.Integration{ReadeckEnabled: true}, []string{"readeck"}},
		{"raindrop", model.Integration{RaindropEnabled: true}, []string{"raindrop"}},
		{"both", model.Integration{ReadeckEnabled: true, RaindropEnabled: true}, []string{"readeck", "raindrop"}},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			var result []string
			for _, destination := range Destinations(&scenario.settings) {
				result = append(result, destination.ID)
			}

			if !slices.Equal(result, scenario.expected) {
				t.Errorf("expected %v, got %v", scenario.expected, result)
			}
		})
	}
}

func TestSendEntryToRefusesDestinationsThatAreNotEnabled(t *testing.T) {
	entry := &model.Entry{URL: "https://example.org/post", Title: "Post"}

	for _, destination := range []string{"readeck", "raindrop", "wallabag", ""} {
		if err := SendEntryTo(entry, &model.Integration{}, destination); !errors.Is(err, ErrDestinationUnavailable) {
			t.Errorf("%q: expected the destination to be unavailable, got %v", destination, err)
		}
	}
}

func TestSendEntryToReportsReadeckFailures(t *testing.T) {
	t.Setenv("INTEGRATION_ALLOW_PRIVATE_NETWORKS", "1")

	options, err := config.NewConfigParser().ParseEnvironmentVariables()
	if err != nil {
		t.Fatal(err)
	}

	previousOptions := config.Opts
	config.Opts = options
	t.Cleanup(func() { config.Opts = previousOptions })

	status := http.StatusCreated
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	}))
	defer server.Close()

	settings := &model.Integration{ReadeckEnabled: true, ReadeckURL: server.URL, ReadeckAPIKey: "key", ReadeckOnlyURL: true}
	entry := &model.Entry{URL: "https://example.org/post", Title: "Post"}

	if err := SendEntryTo(entry, settings, "readeck"); err != nil {
		t.Errorf("expected the entry to be sent, got %v", err)
	}

	status = http.StatusUnauthorized
	if err := SendEntryTo(entry, settings, "readeck"); err == nil {
		t.Error("expected a rejected entry to be reported")
	}
}
