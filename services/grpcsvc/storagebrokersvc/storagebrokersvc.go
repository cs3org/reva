package storagebrokersvc

import (
	"context"
	"fmt"

	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"

	storagebrokerv0alphapb "github.com/cernbox/go-cs3apis/cs3/storagebroker/v0alpha"
	"github.com/cernbox/reva/pkg/err"
	"github.com/cernbox/reva/pkg/log"
	"github.com/cernbox/reva/pkg/storage"
	"github.com/cernbox/reva/pkg/storage/broker/static"
	"github.com/mitchellh/mapstructure"
)

var logger = log.New("storagebrokersvc")
var errors = err.New("storagebrokersvc")

type service struct {
	broker storage.Broker
}
type config struct {
	Driver string                 `mapstructure:"driver"`
	Static map[string]interface{} `mapstructure:"static"`
}

// New creates a new StorageBrokerService
func New(m map[string]interface{}) (storagebrokerv0alphapb.StorageBrokerServiceServer, error) {

	c, err := parseConfig(m)
	if err != nil {
		return nil, errors.Wrap(err, "unable to parse config")
	}

	broker, err := getBroker(c)
	if err != nil {
		return nil, errors.Wrap(err, "unable to init broker")
	}

	service := &service{
		broker: broker,
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

func getBroker(c *config) (storage.Broker, error) {
	switch c.Driver {
	case "static":
		return static.New(c.Static)
	default:
		return nil, fmt.Errorf("driver not found: %s", c.Driver)
	}
}
func (s *service) Find(ctx context.Context, req *storagebrokerv0alphapb.FindRequest) (*storagebrokerv0alphapb.FindResponse, error) {
	fn := req.Filename
	p, err := s.broker.FindProvider(ctx, fn)
	if err != nil {
		logger.Error(ctx, err)
		res := &storagebrokerv0alphapb.FindResponse{
			Status: &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL},
		}
		return res, nil
	}

	provider := format(p)
	res := &storagebrokerv0alphapb.FindResponse{
		Status:       &rpcpb.Status{Code: rpcpb.Code_CODE_OK},
		ProviderInfo: provider,
	}
	return res, nil
}

func format(p *storage.ProviderInfo) *storagebrokerv0alphapb.ProviderInfo {
	return &storagebrokerv0alphapb.ProviderInfo{
		Location: p.Location,
	}
}
