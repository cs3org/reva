package manager

import (
	"context"
	"image"
	"io"

	"github.com/anthonynsimon/bild/imgio"
	"github.com/anthonynsimon/bild/transform"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage/utils/downloader"
	"github.com/pkg/errors"
)

type Config struct {
	GatewaySVC string `mapstructure:"gateway_svc"`
	Quality    int    `mapstructure:"quality"`
}

type Thumbnail struct {
	c          *Config
	downloader downloader.Downloader
}

func NewThumbnail(d downloader.Downloader, c *Config) (*Thumbnail, error) {
	t := &Thumbnail{
		c:          c,
		downloader: d,
	}
	return t, nil
}

type FileType int

const (
	pngType FileType = iota
	jpegType
	bmpType
)

func (t *Thumbnail) GetThumbnail(ctx context.Context, file string, width, height int, outType FileType) (io.ReadCloser, error) {
	// TODO: add cache

	r, err := t.downloader.Download(ctx, file)
	if err != nil {
		return nil, errors.Wrap(err, "error downloading file "+file)
	}
	defer r.Close()

	img, _, err := image.Decode(r)
	if err != nil {
		return nil, errors.Wrap(err, "error decoding file "+file)
	}

	resized := transform.Resize(img, width, height, transform.Linear)
	encoder, err := t.getEncoderByType(outType)
	if err != nil {
		return nil, err
	}

	out, w := io.Pipe()
	go func() {
		defer w.Close()
		encoder(w, resized)
	}()

	return out, nil

}

func (t *Thumbnail) getEncoderByType(ttype FileType) (imgio.Encoder, error) {
	switch ttype {
	case pngType:
		return imgio.PNGEncoder(), nil
	case jpegType:
		return imgio.JPEGEncoder(t.c.Quality), nil
	case bmpType:
		return imgio.BMPEncoder(), nil
	}
	return nil, errtypes.NotSupported("type not supported")
}
