// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package stream // import "miniflux.app/v2/internal/ui/stream"

import (
	"strings"
	"testing"
	"unicode/utf8"

	"miniflux.app/v2/internal/model"
)

func TestSummarizeExtractsReadableText(t *testing.T) {
	excerpt, _ := summarize(`<h2>Heading</h2><p>Some <strong>bold</strong> text, &amp; a <a href="#">link</a>.</p><p>Next paragraph</p>`)
	expected := "Heading Some bold text, & a link. Next paragraph"
	if excerpt != expected {
		t.Fatalf(`Expected %q, got %q`, expected, excerpt)
	}
}

func TestSummarizeSkipsCaptionsCodeAndTables(t *testing.T) {
	excerpt, _ := summarize(`<figure><figcaption>Photo credit</figcaption></figure><pre>let x = 1</pre><table><tr><td>cell</td></tr></table><p>Body</p>`)
	if excerpt != "Body" {
		t.Fatalf(`Expected only the body text, got %q`, excerpt)
	}
}

func TestSummarizeTruncatesAtAWordBoundary(t *testing.T) {
	excerpt, _ := summarize("<p>" + strings.Repeat("word ", 200) + "</p>")

	if !strings.HasSuffix(excerpt, "word…") {
		t.Fatalf(`Expected a whole word before the ellipsis, got %q`, excerpt)
	}
	if utf8.RuneCountInString(excerpt) > excerptMaxRunes+1 {
		t.Fatalf(`Expected at most %d runes, got %d`, excerptMaxRunes+1, utf8.RuneCountInString(excerpt))
	}
}

func TestSummarizeTruncatesTextWithoutSpaces(t *testing.T) {
	excerpt, _ := summarize("<p>" + strings.Repeat("字", 500) + "</p>")
	if utf8.RuneCountInString(excerpt) != excerptMaxRunes+1 {
		t.Fatalf(`Expected %d runes, got %d`, excerptMaxRunes+1, utf8.RuneCountInString(excerpt))
	}
}

func TestSummarizePicksTheFirstUsableImage(t *testing.T) {
	scenarios := []struct {
		name     string
		content  string
		expected string
	}{
		{"first image", `<img src="https://example.org/a.jpg"><img src="https://example.org/b.jpg">`, "https://example.org/a.jpg"},
		{"small image skipped", `<img src="https://example.org/icon.png" width="16" height="16"><img src="https://example.org/b.jpg">`, "https://example.org/b.jpg"},
		{"unknown size kept", `<img src="https://example.org/a.jpg" width="100%">`, "https://example.org/a.jpg"},
		{"data url skipped", `<img src="data:image/png;base64,AAAA">`, ""},
		{"image after the text budget", `<p>` + strings.Repeat("word ", 2000) + `</p><img src="https://example.org/late.jpg">`, "https://example.org/late.jpg"},
		{"no image", `<p>Text only</p>`, ""},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			_, imageURL := summarize(scenario.content)
			if imageURL != scenario.expected {
				t.Fatalf(`Expected %q, got %q`, scenario.expected, imageURL)
			}
		})
	}
}

func TestNewCardsPrefersEnclosureImages(t *testing.T) {
	entries := model.Entries{{
		FeedID:  14,
		Content: `<img src="https://example.org/content.jpg">`,
		Feed:    &model.Feed{Title: "The Example"},
		Enclosures: model.EnclosureList{
			{URL: "https://example.org/episode.mp3", MimeType: "audio/mpeg"},
			{URL: "https://example.org/cover.jpg", MimeType: "image/jpeg"},
		},
	}}

	card := NewCards(entries)[0]

	if card.ImageURL != "https://example.org/cover.jpg" {
		t.Fatalf(`Expected the enclosure image, got %q`, card.ImageURL)
	}
	if card.Initial != "T" {
		t.Fatalf(`Expected initial "T", got %q`, card.Initial)
	}
	if card.Hue != 2 {
		t.Fatalf(`Expected hue 2, got %d`, card.Hue)
	}
}

func TestNewCardsFallsBackForFeedTitlesWithoutLetters(t *testing.T) {
	entries := model.Entries{{Feed: &model.Feed{Title: "!!!"}}}

	if initial := NewCards(entries)[0].Initial; initial != "#" {
		t.Fatalf(`Expected "#", got %q`, initial)
	}
}
