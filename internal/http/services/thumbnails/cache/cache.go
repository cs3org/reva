package cache

import (
	"io"
	"io/ioutil"
)

type Cache interface {
	Get(file string, width, height int) (io.ReadCloser, error)
	Set(file string, width, height int, r io.Reader) error
}

type noCache struct{}

func NewNoCache() Cache {
	return noCache{}
}

func (noCache) Get(_ string, _, _ int) (io.ReadCloser, error) {
	return nil, ErrNotFound{}
}

func (noCache) Set(_ string, _, _ int, r io.Reader) error {
	// consume the reader
	_, _ = io.Copy(ioutil.Discard, r)
	return nil
}
