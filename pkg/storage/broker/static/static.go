package static

import (
	"context"
	"strings"

	"github.com/cernbox/reva/pkg/log"
	"github.com/cernbox/reva/pkg/storage/broker/registry"

	"github.com/cernbox/reva/pkg/storage"
	"github.com/mitchellh/mapstructure"
)

func init() {
	registry.Register("static", New)
}

var logger = log.New("static")

type broker struct {
	rules map[string]string
}

func (b *broker) ListProviders(ctx context.Context) ([]*storage.ProviderInfo, error) {
	providers := []*storage.ProviderInfo{}
	for k, v := range b.rules {
		providers = append(providers, &storage.ProviderInfo{
			Endpoint:  v,
			MountPath: k,
		})
	}
	return providers, nil
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
		MountPath: match,
		Endpoint:  b.rules[match],
	}
	return p, nil
}

type config struct {
	Rules map[string]string `mapstructure:"rules"`
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
