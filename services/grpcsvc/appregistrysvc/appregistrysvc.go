package appregistrysvc

import (
	"context"
	"fmt"

	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"

	appregistryv0alphapb "github.com/cernbox/go-cs3apis/cs3/appregistry/v0alpha"
	"github.com/cernbox/reva/pkg/app"
	"github.com/cernbox/reva/pkg/app/registry/static"
	"github.com/cernbox/reva/pkg/err"
	"github.com/cernbox/reva/pkg/log"
	"github.com/mitchellh/mapstructure"
)

var logger = log.New("appregistry")
var errors = err.New("appregistry")

type service struct {
	registry app.Registry
}
type config struct {
	Driver string                 `mapstructure:"driver"`
	Static map[string]interface{} `mapstructure:"static"`
}

// New creates a new StorageRegistryService
func New(m map[string]interface{}) (appregistryv0alphapb.AppRegistryServiceServer, error) {

	c, err := parseConfig(m)
	if err != nil {
		return nil, errors.Wrap(err, "unable to parse config")
	}

	registry, err := getRegistry(c)
	if err != nil {
		return nil, errors.Wrap(err, "unable to init registry")
	}

	service := &service{
		registry: registry,
	}

	return service, nil
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

func getRegistry(c *config) (app.Registry, error) {
	switch c.Driver {
	case "static":
		return static.New(c.Static)
	default:
		return nil, fmt.Errorf("driver not found: %s", c.Driver)
	}
}
func (s *service) Find(ctx context.Context, req *appregistryv0alphapb.FindRequest) (*appregistryv0alphapb.FindResponse, error) {
	ext := req.FilenameExtension
	mime := req.FilenameMimetype
	p, err := s.registry.FindProvider(ctx, ext, mime)
	if err != nil {
		logger.Error(ctx, err)
		res := &appregistryv0alphapb.FindResponse{
			Status: &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL},
		}
		return res, nil
	}

	provider := format(p)
	res := &appregistryv0alphapb.FindResponse{
		Status:          &rpcpb.Status{Code: rpcpb.Code_CODE_OK},
		AppProviderInfo: provider,
	}
	return res, nil
}

func format(p *app.ProviderInfo) *appregistryv0alphapb.AppProviderInfo {
	return &appregistryv0alphapb.AppProviderInfo{
		Location: p.Location,
	}
}
