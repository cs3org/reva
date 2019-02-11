package appregistrysvc

import (
	"context"
	"fmt"

	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
	"google.golang.org/grpc"

	appregistryv0alphapb "github.com/cernbox/go-cs3apis/cs3/appregistry/v0alpha"
	"github.com/cernbox/reva/cmd/revad/grpcserver"
	"github.com/cernbox/reva/pkg/app"
	"github.com/cernbox/reva/pkg/app/registry/static"
	"github.com/cernbox/reva/pkg/err"
	"github.com/cernbox/reva/pkg/log"
	"github.com/mitchellh/mapstructure"
)

var logger = log.New("appregistrysvc")
var errors = err.New("appregistrysvc")

func init() {
	grpcserver.Register("appregistrysvc", New)
}

type service struct {
	registry app.Registry
}

type config struct {
	Driver string                 `mapstructure:"driver"`
	Static map[string]interface{} `mapstructure:"static"`
}

// New creates a new StorageRegistryService
func New(m map[string]interface{}, ss *grpc.Server) error {

	c, err := parseConfig(m)
	if err != nil {
		return err
	}

	registry, err := getRegistry(c)
	if err != nil {
		return err
	}

	service := &service{
		registry: registry,
	}

	appregistryv0alphapb.RegisterAppRegistryServiceServer(ss, service)
	return nil
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
