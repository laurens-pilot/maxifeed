// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package model // import "miniflux.app/v2/internal/model"

import "time"

// ReadingDay records how many entries a user read from the stream on a calendar day.
type ReadingDay struct {
	Day       time.Time
	ReadCount int
}
