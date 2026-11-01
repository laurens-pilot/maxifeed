// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package stream // import "miniflux.app/v2/internal/ui/stream"

import (
	"strings"
	"testing"

	"miniflux.app/v2/internal/model"
)

func paragraphOf(length int) string {
	return "<p>" + strings.Repeat("a", length) + "</p>"
}

func TestLooksLikeSummary(t *testing.T) {
	scenarios := []struct {
		name     string
		entry    model.Entry
		expected bool
	}{
		{"empty", model.Entry{}, true},
		{"short text", model.Entry{Content: paragraphOf(400)}, true},
		{"markup doesn't count", model.Entry{Content: paragraphOf(400) + `<img src="https://example.org/` + strings.Repeat("a", 4000) + `.png">`}, true},
		{"full article", model.Entry{Content: paragraphOf(6000)}, false},
		{"podcast", model.Entry{Content: paragraphOf(400), Enclosures: model.EnclosureList{{MimeType: "audio/mpeg"}}}, false},
		{"video", model.Entry{Content: paragraphOf(400), Enclosures: model.EnclosureList{{MimeType: "video/mp4"}}}, false},
		{"image", model.Entry{Content: paragraphOf(400), Enclosures: model.EnclosureList{{MimeType: "image/png"}}}, true},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			if result := LooksLikeSummary(&scenario.entry); result != scenario.expected {
				t.Errorf("expected %v, got %v", scenario.expected, result)
			}
		})
	}
}

func TestIsFullText(t *testing.T) {
	scenarios := []struct {
		name     string
		content  string
		summary  string
		expected bool
	}{
		{"much longer article", paragraphOf(8000), paragraphOf(300), true},
		{"article for a title-only post", paragraphOf(3000), "", true},
		{"cookie wall", paragraphOf(600), "", false},
		{"paywall teaser", paragraphOf(1300), paragraphOf(1200), false},
		{"same text", paragraphOf(1800), paragraphOf(1800), false},
		{"nothing extracted", "", paragraphOf(300), false},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			if result := IsFullText(scenario.content, scenario.summary); result != scenario.expected {
				t.Errorf("expected %v, got %v", scenario.expected, result)
			}
		})
	}
}
