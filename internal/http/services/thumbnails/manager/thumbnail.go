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
	"bytes"
	"context"
	"fmt"
	"image"

	"github.com/cs3org/reva/internal/http/services/thumbnails/cache"
	"github.com/cs3org/reva/internal/http/services/thumbnails/cache/registry"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/mime"
	"github.com/cs3org/reva/pkg/storage/utils/downloader"
	"github.com/disintegration/imaging"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"

	// load all the cache drivers
	_ "github.com/cs3org/reva/internal/http/services/thumbnails/cache/loader"
)

// FileType is the output format of the thumbnail
type FileType int

const (
	PNGType FileType = iota
	JPEGType
	BMPType
)

// Config is the config for the Thumbnail service
type Config struct {
	Quality      int
	Resolutions  []string
	Cache        string
	CacheDrivers map[string]map[string]interface{}
}

// Thumbnail is the service that generates thumbnails
type Thumbnail struct {
	c           *Config
	downloader  downloader.Downloader
	cache       cache.Cache
	log         *zerolog.Logger
	resolutions Resolutions
}

// NewThumbnail creates a new Thumbnail service that generates thumbnails
func NewThumbnail(d downloader.Downloader, c *Config, log *zerolog.Logger) (*Thumbnail, error) {
	res, err := ParseResolutions(c.Resolutions)
	if err != nil {
		return nil, errors.Wrap(err, "thumbnails: error parsing resolutions")
	}
	t := &Thumbnail{
		c:           c,
		downloader:  d,
		log:         log,
		resolutions: res,
	}
	err = t.initCache()
	if err != nil {
		return nil, errors.Wrap(err, "thumbnails: error initting the cache")
	}
	return t, nil
}

// GetThumbnail generate a thumbnail from the file, returning the thumb and the mimetype of the thumb.
// The mimetype depends on the out type (PNG, JPEG, BMP).
// If a cache is enabled in the configuration, it will first check if the file with the given etag
// was already generated and saved into the cache.
func (t *Thumbnail) GetThumbnail(ctx context.Context, file, etag string, width, height int, outType FileType) ([]byte, string, error) {
	log := t.log.With().Str("file", file).Str("etag", etag).Int("width", width).Int("height", height).Logger()
	if d, err := t.cache.Get(file, etag, width, height); err == nil {
		log.Debug().Msg("thumbnails: cache hit")
		return d, "", nil
	}

	log.Debug().Msg("thumbnails: cache miss")

	// the thumbnail was not found in the cache
	r, err := t.downloader.Download(ctx, file)
	if err != nil {
		return nil, "", errors.Wrap(err, "thumbnails: error downloading file "+file)
	}
	defer r.Close()

	img, _, err := image.Decode(r)
	if err != nil {
		return nil, "", errors.Wrap(err, "thumbnails: error decoding file "+file)
	}

	resolution := image.Rect(0, 0, width, height)
	match := t.resolutions.ClosestMatch(resolution, img.Bounds())
	thumb := imaging.Thumbnail(img, match.Dx(), match.Dy(), imaging.Linear)

	var buf bytes.Buffer
	format, opts := t.getEncoderFormat(outType)
	err = imaging.Encode(&buf, thumb, format, opts...)
	if err != nil {
		return nil, "", errors.Wrap(err, "thumbnails: error encoding image")
	}

	data := buf.Bytes()
	err = t.cache.Set(file, etag, width, height, data)
	if err != nil {
		log.Warn().Msg("failed to save data into the cache")
	} else {
		t.log.Debug().Msg("saved thumbnail into cache")
	}

	return data, getMimeType(outType), nil
}

func getMimeType(ttype FileType) string {
	switch ttype {
	case PNGType:
		return mime.Detect(false, ".png")
	case BMPType:
		return mime.Detect(false, ".bmp")
	default:
		return mime.Detect(false, ".jpg")
	}
}

func (t *Thumbnail) getEncoderFormat(ttype FileType) (imaging.Format, []imaging.EncodeOption) {
	switch ttype {
	case PNGType:
		return imaging.PNG, nil
	case BMPType:
		return imaging.BMP, nil
	default:
		return imaging.JPEG, []imaging.EncodeOption{imaging.JPEGQuality(t.c.Quality)}
	}
}

func (t *Thumbnail) initCache() error {
	if t.c.Cache == "" {
		t.cache = cache.NewNoCache()
		return nil
	}
	f, ok := registry.NewFuncs[t.c.Cache]
	if !ok {
		return errtypes.NotFound(fmt.Sprintf("driver %s not found for thumbnails cache", t.c.Cache))
	}
	c, ok := t.c.CacheDrivers[t.c.Cache]
	if !ok {
		// if the user did not provide the config
		// just use an empty config
		c = make(map[string]interface{})
	}
	cache, err := f(c)
	if err != nil {
		return err
	}
	t.cache = cache
	return nil
}
