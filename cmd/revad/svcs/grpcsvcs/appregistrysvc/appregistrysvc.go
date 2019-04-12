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
func (s *service) GetAppProvider(ctx context.Context, req *appregistryv0alphapb.GetAppProviderRequest) (*appregistryv0alphapb.GetAppProviderResponse, error) {
	mime := req.MimeType
	p, err := s.registry.FindProvider(ctx, mime)
	if err != nil {
		logger.Error(ctx, err)
		res := &appregistryv0alphapb.GetAppProviderResponse{
			Status: &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL},
		}
		return res, nil
	}

	provider := format(p)
	res := &appregistryv0alphapb.GetAppProviderResponse{
		Status:   &rpcpb.Status{Code: rpcpb.Code_CODE_OK},
		Provider: provider,
	}
	return res, nil
}

func (s *service) ListAppProviders(ctx context.Context, req *appregistryv0alphapb.ListAppProvidersRequest) (*appregistryv0alphapb.ListAppProvidersResponse, error) {
	pvds, err := s.registry.ListProviders(ctx)
	if err != nil {
		res := &appregistryv0alphapb.ListAppProvidersResponse{
			Status: &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL},
		}
		return res, nil
	}
	var providers []*appregistryv0alphapb.ProviderInfo
	for _, pvd := range pvds {
		providers = append(providers, format(pvd))
	}

	res := &appregistryv0alphapb.ListAppProvidersResponse{
		Status:    &rpcpb.Status{Code: rpcpb.Code_CODE_OK},
		Providers: providers,
	}
	return res, nil
}

func format(p *app.ProviderInfo) *appregistryv0alphapb.ProviderInfo {
	return &appregistryv0alphapb.ProviderInfo{
		Address: p.Location,
	}
}
