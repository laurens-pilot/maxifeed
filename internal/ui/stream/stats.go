// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package stream // import "miniflux.app/v2/internal/ui/stream"

import (
	"time"

	"miniflux.app/v2/internal/model"
)

// Stats summarizes the user's reading habit.
type Stats struct {
	Streak    int
	ReadToday int
}

// NextStreak returns what the streak becomes once the user reads something today.
func (s Stats) NextStreak() int {
	if s.ReadToday > 0 {
		return s.Streak
	}
	return s.Streak + 1
}

// NewStats computes the stats from reading days sorted from newest to oldest.
// A streak stays alive until the end of the day after the last reading day.
func NewStats(days []model.ReadingDay, today time.Time) Stats {
	var stats Stats
	if len(days) == 0 {
		return stats
	}

	expected := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	if days[0].Day.Equal(expected) {
		stats.ReadToday = days[0].ReadCount
	} else {
		expected = expected.AddDate(0, 0, -1)
	}

	for _, day := range days {
		if !day.Day.Equal(expected) {
			break
		}
		stats.Streak++
		expected = expected.AddDate(0, 0, -1)
	}

	return stats
}
