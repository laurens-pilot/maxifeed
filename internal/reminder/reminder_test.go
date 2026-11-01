// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package reminder // import "miniflux.app/v2/internal/reminder"

import (
	"strings"
	"testing"

	"miniflux.app/v2/internal/locale"
	"miniflux.app/v2/internal/ui/stream"
)

func TestComposeMentionsTheStreakOnlyWhenItIsAtRisk(t *testing.T) {
	printer := locale.NewPrinter("en_US")

	scenarios := []struct {
		name     string
		stats    stream.Stats
		expected string
	}{
		{"no streak", stream.Stats{}, "Your feed is ready"},
		{"streak at risk", stream.Stats{Streak: 5}, "Keep your 5-day streak going"},
		{"already read today", stream.Stats{Streak: 5, ReadToday: 3}, "Your feed is ready"},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			result := compose(printer, scenario.stats, 12, "Dutch election results", "")

			if result.Title != scenario.expected {
				t.Errorf("unexpected title %q", result.Title)
			}

			if result.Body != "12 waiting, starting with: Dutch election results" {
				t.Errorf("unexpected body %q", result.Body)
			}
		})
	}
}

func TestComposeShortensLongHeadlines(t *testing.T) {
	result := compose(locale.NewPrinter("en_US"), stream.Stats{}, 1, strings.Repeat("é", 500), "")

	if !strings.HasSuffix(result.Body, strings.Repeat("é", maxHeadlineLength)+"…") {
		t.Errorf("unexpected body %q", result.Body)
	}
}

func TestComposeMentionsTheArticleInProgress(t *testing.T) {
	printer := locale.NewPrinter("en_US")

	waiting := compose(printer, stream.Stats{}, 12, "Dutch election results", "A very long read")
	if !strings.Contains(waiting.Body, "12") || !strings.Contains(waiting.Body, "A very long read") {
		t.Errorf("unexpected body %q", waiting.Body)
	}

	caughtUp := compose(printer, stream.Stats{}, 0, "", "A very long read")
	if !strings.Contains(caughtUp.Body, "A very long read") || strings.Contains(caughtUp.Body, "0") {
		t.Errorf("unexpected body %q", caughtUp.Body)
	}
}
