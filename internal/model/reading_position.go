// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package model // import "miniflux.app/v2/internal/model"

type ReadingPosition struct {
	Block  int `json:"block"`
	Blocks int `json:"blocks"`
}

func (p *ReadingPosition) IsValid() bool {
	return p.Block >= 0 && p.Blocks > 0 && p.Block <= p.Blocks
}

func (p *ReadingPosition) Fraction() float64 {
	return float64(p.Block) / float64(p.Blocks)
}
