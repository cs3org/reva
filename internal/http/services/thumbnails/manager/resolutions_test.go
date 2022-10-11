// Copyright 2018-2021 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package manager

import (
	"image"
	"testing"
)

func TestResize(t *testing.T) {
	tests := []struct {
		name      string
		requested image.Rectangle
		source    image.Rectangle
		expected  image.Rectangle
	}{
		{
			name:      "source and requested with same size",
			requested: image.Rect(0, 0, 1920, 1080),
			source:    image.Rect(0, 0, 1920, 1080),
			expected:  image.Rect(0, 0, 1920, 1080),
		},
		{
			name:      "same ratio",
			requested: image.Rect(0, 0, 1280, 720),
			source:    image.Rect(0, 0, 1920, 1080),
			expected:  image.Rect(0, 0, 1280, 720),
		},
		{
			name:      "source bigger ratio",
			requested: image.Rect(0, 0, 1280, 720),
			source:    image.Rect(0, 0, 1920, 920),
			expected:  image.Rect(0, 0, 1280, 613),
		},
		{
			name:      "source height < requested height - source ratio > requested ratio",
			requested: image.Rect(0, 0, 1080, 1920),
			source:    image.Rect(0, 0, 1920, 1080),
			expected:  image.Rect(0, 0, 1080, 607),
		},
		{
			name:      "source height > requested height - source ratio < requested ratio",
			requested: image.Rect(0, 0, 1080, 600),
			source:    image.Rect(0, 0, 1920, 1080),
			expected:  image.Rect(0, 0, 1066, 600),
		},
		{
			name:      "portrait source",
			requested: image.Rect(0, 0, 1280, 1280),
			source:    image.Rect(0, 0, 1080, 1920),
			expected:  image.Rect(0, 0, 720, 1280),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			res := resize(tt.requested, tt.source)
			if res.Dx() != tt.expected.Dx() || res.Dy() != tt.expected.Dy() {
				t.Fatalf("resize() failed: requested=%+v source=%+v expected=%+v got=%+v", tt.requested, tt.source, tt.expected, res)
			}

		})

	}
}

func TestMatchOrResize(t *testing.T) {
	tests := []struct {
		name             string
		matchResolutions Resolutions
		requested        image.Rectangle
		source           image.Rectangle
		expected         image.Rectangle
	}{
		{
			name:             "matched",
			matchResolutions: []image.Rectangle{image.Rect(0, 0, 64, 64)},
			requested:        image.Rect(0, 0, 64, 64),
			source:           image.Rect(0, 0, 1920, 1080),
			expected:         image.Rect(0, 0, 64, 64),
		},
		{
			name:      "requested bigger than source - landscape",
			requested: image.Rect(0, 0, 5000, 5000),
			source:    image.Rect(0, 0, 1920, 1080),
			expected:  image.Rect(0, 0, 1920, 1080),
		},
		{
			name:      "requested bigger than source - portrait",
			requested: image.Rect(0, 0, 5000, 5000),
			source:    image.Rect(0, 0, 1080, 1920),
			expected:  image.Rect(0, 0, 1080, 1920),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			res := tt.matchResolutions.MatchOrResize(tt.requested, tt.source)
			if res.Dx() != tt.expected.Dx() || res.Dy() != tt.expected.Dy() {
				t.Fatalf("resize() failed: requested=%+v source=%+v expected=%+v got=%+v", tt.requested, tt.source, tt.expected, res)
			}

		})

	}
}
