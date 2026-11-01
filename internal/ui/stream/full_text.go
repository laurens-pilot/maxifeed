// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package stream // import "miniflux.app/v2/internal/ui/stream"

import (
	"strings"
	"unicode/utf8"

	"miniflux.app/v2/internal/model"
	"miniflux.app/v2/internal/reader/sanitizer"
)

const (
	summaryMaxLength  = 2500
	fullTextMinLength = 1200
	fullTextMinGrowth = 1.5
)

func LooksLikeSummary(entry *model.Entry) bool {
	return textLength(entry.Content) < summaryMaxLength && !entry.Enclosures.ContainsAudioOrVideo()
}

func IsFullText(content, summary string) bool {
	length := textLength(content)
	return length >= fullTextMinLength && float64(length) >= float64(textLength(summary))*fullTextMinGrowth
}

func textLength(content string) int {
	return utf8.RuneCountInString(strings.TrimSpace(sanitizer.StripTags(content)))
}
