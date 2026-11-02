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

func TestSavedCardsReportProgress(t *testing.T) {
	entries := model.Entries{
		{ID: 1, ReadingTime: 20, Feed: &model.Feed{Title: "Started"}},
		{ID: 2, ReadingTime: 12, Feed: &model.Feed{Title: "New"}},
		{ID: 3, ReadingTime: 0, Feed: &model.Feed{Title: "No estimate"}},
		{ID: 4, ReadingTime: 10, Feed: &model.Feed{Title: "At the end"}},
	}
	positions := map[int64]*model.ReadingPosition{
		1: {Block: 25, Blocks: 100},
		3: {Block: 1, Blocks: 2},
		4: {Block: 50, Blocks: 50},
	}

	cards := NewSavedCards(entries, positions)

	scenarios := []struct {
		started bool
		percent int
		minutes int
	}{
		{true, 25, 15},
		{false, 0, 12},
		{true, 50, 0},
		{true, 100, 1},
	}

	for index, scenario := range scenarios {
		card := cards[index]
		if card.Started() != scenario.started || card.Percent() != scenario.percent || card.MinutesLeft() != scenario.minutes {
			t.Errorf("card %d: got started=%v percent=%d minutes=%d", index, card.Started(), card.Percent(), card.MinutesLeft())
		}
	}

	started, notStarted := SplitStarted(cards)
	if len(started) != 3 || len(notStarted) != 1 || notStarted[0].Entry.ID != 2 {
		t.Errorf("unexpected split: %d started, %d not started", len(started), len(notStarted))
	}
}

func TestExcerptSkipsAsides(t *testing.T) {
	card := newCard(&model.Entry{
		Content: `<aside><p>Related articles</p><p>View more</p></aside><p>The article starts here.</p>`,
		Feed:    &model.Feed{Title: "A feed"},
	})

	if card.Excerpt != "The article starts here." {
		t.Errorf("unexpected excerpt %q", card.Excerpt)
	}
}
