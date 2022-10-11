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
	"fmt"
	"image"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// inspired by https://github.com/owncloud/ocis/blob/master/services/thumbnails/pkg/thumbnail/resolutions.go

const (
	_resolutionSeparator = "x"
)

// ParseResolution returns an image.Rectangle representing the resolution given as a string
func ParseResolution(s string) (image.Rectangle, error) {
	parts := strings.Split(s, _resolutionSeparator)
	if len(parts) != 2 {
		return image.Rectangle{}, fmt.Errorf("failed to parse resolution: %s. Expected format <width>x<height>", s)
	}
	width, err := strconv.Atoi(parts[0])
	if err != nil {
		return image.Rectangle{}, fmt.Errorf("width: %s has an invalid value. Expected an integer", parts[0])
	}
	height, err := strconv.Atoi(parts[1])
	if err != nil {
		return image.Rectangle{}, fmt.Errorf("height: %s has an invalid value. Expected an integer", parts[1])
	}
	return image.Rect(0, 0, width, height), nil
}

// Resolutions is a list of image.Rectangle representing resolutions.
type Resolutions []image.Rectangle

// ParseResolutions creates an instance of Resolutions from resolution strings.
func ParseResolutions(strs []string) (Resolutions, error) {
	rs := make(Resolutions, 0, len(strs))
	for _, s := range strs {
		r, err := ParseResolution(s)
		if err != nil {
			return nil, errors.Wrap(err, "could not parse resolutions")
		}
		rs = append(rs, r)
	}
	return rs, nil
}

// MatchOrResize returns the resolution which match one of the resolution, or
// a resolution that fits in the requested resolution, keeping the ratio of
// the source image.
func (rs Resolutions) MatchOrResize(requested, source image.Rectangle) image.Rectangle {
	if r, ok := rs.match(requested); ok {
		return r
	}

	resized := resize(requested, source)
	if isSmaller(resized, source) {
		return resized
	}
	return source
}

func isSmaller(r1, r2 image.Rectangle) bool {
	return r1.Dx() <= r2.Dx() && r1.Dy() <= r2.Dy()
}

func resize(requested, source image.Rectangle) image.Rectangle {
	r := float64(requested.Dx()) / float64(source.Dx())
	dx, dy := requested.Dx(), int(float64(source.Dy())*r)

	if dy <= requested.Dy() {
		return image.Rect(0, 0, dx, dy)
	}

	r = float64(requested.Dy()) / float64(source.Dy())
	dx, dy = int(float64(source.Dx())*r), requested.Dy()
	return image.Rect(0, 0, dx, dy)
}

func (rs Resolutions) match(requested image.Rectangle) (image.Rectangle, bool) {
	for _, r := range rs {
		if r.Dx() == requested.Dx() && r.Dy() == requested.Dy() {
			return r, true
		}
	}
	return image.Rectangle{}, false
}
