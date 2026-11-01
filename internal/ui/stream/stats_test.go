// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package stream // import "miniflux.app/v2/internal/ui/stream"

import (
	"testing"
	"time"

	"miniflux.app/v2/internal/model"
)

func readingDay(date string, count int) model.ReadingDay {
	day, err := time.Parse(time.DateOnly, date)
	if err != nil {
		panic(err)
	}
	return model.ReadingDay{Day: day, ReadCount: count}
}

func TestNextStreak(t *testing.T) {
	if streak := (Stats{Streak: 3, ReadToday: 2}).NextStreak(); streak != 3 {
		t.Fatalf(`Expected reading again today to keep the streak at 3, got %d`, streak)
	}
	if streak := (Stats{Streak: 3}).NextStreak(); streak != 4 {
		t.Fatalf(`Expected the first read of the day to extend the streak to 4, got %d`, streak)
	}
	if streak := (Stats{}).NextStreak(); streak != 1 {
		t.Fatalf(`Expected the first read ever to start a streak of 1, got %d`, streak)
	}
}

func TestNewStats(t *testing.T) {
	today := time.Date(2026, time.March, 1, 22, 30, 0, 0, time.FixedZone("CET", 3600))

	scenarios := []struct {
		name     string
		days     []model.ReadingDay
		expected Stats
	}{
		{"no reading yet", nil, Stats{}},
		{"read today only", []model.ReadingDay{readingDay("2026-03-01", 4)}, Stats{Streak: 1, ReadToday: 4}},
		{"streak across a month boundary", []model.ReadingDay{
			readingDay("2026-03-01", 2),
			readingDay("2026-02-28", 9),
			readingDay("2026-02-27", 1),
		}, Stats{Streak: 3, ReadToday: 2}},
		{"streak survives until tomorrow", []model.ReadingDay{
			readingDay("2026-02-28", 9),
			readingDay("2026-02-27", 1),
		}, Stats{Streak: 2}},
		{"missed yesterday restarts the streak", []model.ReadingDay{
			readingDay("2026-03-01", 3),
			readingDay("2026-02-27", 1),
		}, Stats{Streak: 1, ReadToday: 3}},
		{"streak is over", []model.ReadingDay{readingDay("2026-02-20", 5)}, Stats{}},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			if stats := NewStats(scenario.days, today); stats != scenario.expected {
				t.Fatalf(`Expected %+v, got %+v`, scenario.expected, stats)
			}
		})
	}
}
