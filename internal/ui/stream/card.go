// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package stream // import "miniflux.app/v2/internal/ui/stream"

import (
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"

	"miniflux.app/v2/internal/model"
)

const (
	excerptMaxRunes = 280
	textBudgetBytes = excerptMaxRunes * 8
	minImageSize    = 120
	hueCount        = 12
)

var (
	inlineTags  = []string{"a", "abbr", "b", "cite", "code", "em", "i", "mark", "s", "small", "span", "strong", "sub", "sup", "time", "u"}
	skippedTags = []string{"figcaption", "pre", "table"}
)

// Card is an entry prepared for display in the stream.
type Card struct {
	Entry    *model.Entry
	ImageURL string
	Excerpt  string
	Initial  string
	Hue      int64
}

// NewCards builds a card for each entry, keeping their order.
func NewCards(entries model.Entries) []*Card {
	cards := make([]*Card, 0, len(entries))
	for _, entry := range entries {
		cards = append(cards, newCard(entry))
	}
	return cards
}

func newCard(entry *model.Entry) *Card {
	excerpt, contentImage := summarize(entry.Content)

	imageURL := enclosureImage(entry.Enclosures)
	if imageURL == "" {
		imageURL = contentImage
	}

	return &Card{
		Entry:    entry,
		ImageURL: imageURL,
		Excerpt:  excerpt,
		Initial:  initial(entry.Feed.Title),
		Hue:      entry.FeedID % hueCount,
	}
}

func enclosureImage(enclosures model.EnclosureList) string {
	for _, enclosure := range enclosures {
		if enclosure.IsImage() && isWebURL(enclosure.URL) {
			return enclosure.URL
		}
	}
	return ""
}

func initial(title string) string {
	for _, character := range title {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			return string(unicode.ToUpper(character))
		}
	}
	return "#"
}

func summarize(content string) (excerpt, imageURL string) {
	tokenizer := html.NewTokenizer(strings.NewReader(content))
	var text strings.Builder
	skippedDepth := 0

	for imageURL == "" || text.Len() < textBudgetBytes {
		if tokenizer.Next() == html.ErrorToken {
			break
		}

		token := tokenizer.Token()
		switch token.Type {
		case html.TextToken:
			if skippedDepth == 0 && text.Len() < textBudgetBytes {
				text.WriteString(token.Data)
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			if token.Data == "img" && imageURL == "" {
				imageURL = imageSource(token)
			}
			if token.Type == html.StartTagToken && slices.Contains(skippedTags, token.Data) {
				skippedDepth++
			}
			if !slices.Contains(inlineTags, token.Data) {
				text.WriteByte(' ')
			}
		case html.EndTagToken:
			if skippedDepth > 0 && slices.Contains(skippedTags, token.Data) {
				skippedDepth--
			}
			if !slices.Contains(inlineTags, token.Data) {
				text.WriteByte(' ')
			}
		}
	}

	return truncateAtWord(text.String()), imageURL
}

func imageSource(token html.Token) string {
	source := ""
	for _, attribute := range token.Attr {
		switch attribute.Key {
		case "src":
			source = attribute.Val
		case "width", "height":
			if size, err := strconv.Atoi(attribute.Val); err == nil && size < minImageSize {
				return ""
			}
		}
	}

	if isWebURL(source) {
		return source
	}
	return ""
}

func isWebURL(link string) bool {
	return strings.HasPrefix(link, "https://") || strings.HasPrefix(link, "http://")
}

func truncateAtWord(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(text) <= excerptMaxRunes {
		return text
	}

	cut := string([]rune(text)[:excerptMaxRunes])
	if lastSpace := strings.LastIndexByte(cut, ' '); lastSpace > 0 {
		cut = cut[:lastSpace]
	}
	return strings.TrimRight(cut, " ,;:.-–—") + "…"
}
