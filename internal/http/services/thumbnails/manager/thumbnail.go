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

	_ "github.com/cs3org/reva/internal/http/services/thumbnails/cache/loader"
)

type FileType int

const (
	PNGType FileType = iota
	JPEGType
	BMPType
)

type Config struct {
	GatewaySVC   string                            `mapstructure:"gateway_svc"`
	Quality      int                               `mapstructure:"quality"`
	Resolutions  []string                          `mapstructure:"quality"`
	Cache        bool                              `mapstructure:"cache"`
	CacheDriver  string                            `mapstructure:"cache_driver"`
	CacheDrivers map[string]map[string]interface{} `mapstructure:"cache_drivers"`
}

type Thumbnail struct {
	c           *Config
	downloader  downloader.Downloader
	cache       cache.Cache
	log         *zerolog.Logger
	resolutions Resolutions
}

func NewThumbnail(d downloader.Downloader, c *Config, log *zerolog.Logger) (*Thumbnail, error) {
	t := &Thumbnail{
		c:          c,
		downloader: d,
		log:        log,
	}
	err := t.initCache()
	if err != nil {
		return nil, errors.Wrap(err, "thumbnails: error initting the cache")
	}
	return t, nil
}

func (t *Thumbnail) GetThumbnail(ctx context.Context, file string, width, height int, outType FileType) ([]byte, string, error) {
	if d, err := t.cache.Get(file, width, height); err == nil {
		t.log.Debug().Str("file", file).Int("width", width).Int("height", height).Msg("thumbnails: cache hit")
		return d, "", nil
	}

	t.log.Debug().Str("file", file).Int("width", width).Int("height", height).Msg("thumbnails: cache miss")

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
	err = t.cache.Set(file, width, height, data)
	if err != nil {
		t.log.Warn().Str("file", file).Int("width", width).Int("height", height).Err(err).Msg("failed to save data into the cache")
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
	if !t.c.Cache {
		t.cache = cache.NewNoCache()
		return nil
	}
	f, ok := registry.NewFuncs[t.c.CacheDriver]
	if !ok {
		return errtypes.NotFound(fmt.Sprintf("driver %s not found for thumbnails cache", t.c.CacheDriver))
	}
	c, ok := t.c.CacheDrivers[t.c.CacheDriver]
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
