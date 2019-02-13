package storagebrokersvc

import (
	"context"
	"fmt"

	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
	"google.golang.org/grpc"

	storagebrokerv0alphapb "github.com/cernbox/go-cs3apis/cs3/storagebroker/v0alpha"
	"github.com/cernbox/reva/cmd/revad/grpcserver"
	"github.com/cernbox/reva/pkg/err"
	"github.com/cernbox/reva/pkg/log"
	"github.com/cernbox/reva/pkg/storage"
	"github.com/cernbox/reva/pkg/storage/broker/registry"
	"github.com/mitchellh/mapstructure"
)

var logger = log.New("storagebrokersvc")
var errors = err.New("storagebrokersvc")

func init() {
	grpcserver.Register("storagebrokersvc", New)
}

type service struct {
	broker storage.Broker
}

type config struct {
	Driver  string                            `mapstructure:"driver"`
	Drivers map[string]map[string]interface{} `mapstructure:"drivers"`
}

// New creates a new StorageBrokerService
func New(m map[string]interface{}, ss *grpc.Server) error {
	c, err := parseConfig(m)
	if err != nil {
		return err
	}

	broker, err := getBroker(c)
	if err != nil {
		return err
	}

	service := &service{
		broker: broker,
	}

	storagebrokerv0alphapb.RegisterStorageBrokerServiceServer(ss, service)
	return nil
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

func getBroker(c *config) (storage.Broker, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(c.Drivers[c.Driver])
	}
	return nil, fmt.Errorf("driver not found: %s", c.Driver)
}

func (s *service) Discover(ctx context.Context, req *storagebrokerv0alphapb.DiscoverRequest) (*storagebrokerv0alphapb.DiscoverResponse, error) {
	providers := []*storagebrokerv0alphapb.StorageProvider{}
	pinfos, err := s.broker.ListProviders(ctx)
	if err != nil {
		res := &storagebrokerv0alphapb.DiscoverResponse{
			Status: &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL},
		}
		return res, nil
	}

	for _, info := range pinfos {
		providers = append(providers, format(info))
	}

	res := &storagebrokerv0alphapb.DiscoverResponse{
		Status:           &rpcpb.Status{Code: rpcpb.Code_CODE_OK},
		StorageProviders: providers,
	}
	return res, nil
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
		Status:          &rpcpb.Status{Code: rpcpb.Code_CODE_OK},
		StorageProvider: provider,
	}
	return res, nil
}

func format(p *storage.ProviderInfo) *storagebrokerv0alphapb.StorageProvider {
	return &storagebrokerv0alphapb.StorageProvider{
		Endpoint:  p.Endpoint,
		MountPath: p.MountPath,
	}
}
