package demo

import (
	"context"
	"fmt"

	"github.com/cernbox/reva/pkg/app"
	"github.com/cernbox/reva/pkg/log"

	"github.com/mitchellh/mapstructure"
)

var logger = log.New("demo")

type provider struct {
	iframeUIProvider string
}

func (p *provider) GetIFrame(ctx context.Context, filename, mimetype, token string) (string, error) {
	msg := fmt.Sprintf("<iframe src=%s/open/%s?access-token=%s />", p.iframeUIProvider, filename, token)
	return msg, nil
}

type config struct {
	IFrameUIProvider string `mapstructure:"iframe_ui_provider"`
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

// New returns an implementation to of the storage.FS interface that talk to
// a local filesystem.
func New(m map[string]interface{}) (app.Provider, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	return &provider{iframeUIProvider: c.IFrameUIProvider}, nil
}

type notFoundError string

func (e notFoundError) Error() string { return string(e) }
func (e notFoundError) IsNotFound()   {}
