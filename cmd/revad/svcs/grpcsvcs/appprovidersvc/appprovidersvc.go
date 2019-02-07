package appprovidersvc

import (
	"context"
	"fmt"

	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"

	appproviderv0alphapb "github.com/cernbox/go-cs3apis/cs3/appprovider/v0alpha"
	"github.com/cernbox/reva/pkg/app"
	"github.com/cernbox/reva/pkg/app/provider/demo"
	"github.com/cernbox/reva/pkg/err"
	"github.com/cernbox/reva/pkg/log"
	"github.com/mitchellh/mapstructure"
)

var logger = log.New("appregistry")
var errors = err.New("appregistry")

type service struct {
	provider app.Provider
}
type config struct {
	Driver string                 `mapstructure:"driver"`
	Demo   map[string]interface{} `mapstructure:"demo"`
}

// New creates a new StorageRegistryService
func New(m map[string]interface{}) (appproviderv0alphapb.AppProviderServiceServer, error) {

	c, err := parseConfig(m)
	if err != nil {
		return nil, errors.Wrap(err, "unable to parse config")
	}

	provider, err := getProvider(c)
	if err != nil {
		return nil, errors.Wrap(err, "unable to init registry")
	}

	service := &service{
		provider: provider,
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

func getProvider(c *config) (app.Provider, error) {
	switch c.Driver {
	case "demo":
		return demo.New(c.Demo)
	default:
		return nil, fmt.Errorf("driver not found: %s", c.Driver)
	}
}
func (s *service) GetIFrame(ctx context.Context, req *appproviderv0alphapb.GetIFrameRequest) (*appproviderv0alphapb.GetIFrameResponse, error) {

	fn := req.Filename
	mime := req.Miemtype
	token := req.AccessToken

	s.provider.GetIFrame(ctx, fn, mime, token)
	iframeLocation, err := s.provider.GetIFrame(ctx, fn, mime, token)
	if err != nil {
		logger.Error(ctx, err)
		res := &appproviderv0alphapb.GetIFrameResponse{
			Status: &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL},
		}
		return res, nil
	}

	res := &appproviderv0alphapb.GetIFrameResponse{
		Status:         &rpcpb.Status{Code: rpcpb.Code_CODE_OK},
		IframeLocation: iframeLocation,
	}
	return res, nil
}
