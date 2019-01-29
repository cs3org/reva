package static

import (
	"context"
	"strings"

	"github.com/cernbox/reva/pkg/log"

	"github.com/cernbox/reva/pkg/storage"
	"github.com/mitchellh/mapstructure"
)

var logger = log.New("static")

type broker struct {
	rules map[string]string
}

func (b *broker) FindProvider(ctx context.Context, fn string) (*storage.ProviderInfo, error) {
	// find longest match
	var match string
	for prefix := range b.rules {
		if strings.HasPrefix(fn, prefix) && len(prefix) > len(match) {
			match = prefix
		}
	}

	if match == "" {
		return nil, notFoundError("storage provider not found for path " + fn)
	}

	p := &storage.ProviderInfo{
		Location: b.rules[match],
	}
	return p, nil
}

type config struct {
	Rules map[string]string
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
func New(m map[string]interface{}) (storage.Broker, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	return &broker{rules: c.Rules}, nil
}

type notFoundError string

func (e notFoundError) Error() string { return string(e) }
func (e notFoundError) IsNotFound()   {}
