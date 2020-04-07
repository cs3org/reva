package inviteprovider

import (
	"context"
	"fmt"
	inviteApi "github.com/cs3org/go-cs3apis/cs3/invite/v1beta1"
	"github.com/cs3org/reva/pkg/ocm/invite"
	"github.com/cs3org/reva/pkg/ocm/invite/manager/registry"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("inviteprovider", New)
}


type config struct {
	Driver  string                            `mapstructure:"driver"`
	Drivers map[string]map[string]interface{} `mapstructure:"drivers"`
}

type service struct {
	conf *config
	im   invite.Manager
}

func getInviteManager(c *config) (invite.Manager, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(c.Drivers[c.Driver])
	}
	return nil, fmt.Errorf("driver not found: %s", c.Driver)
}

func (s *service) Close() error {
	return nil
}

func (s *service) UnprotectedEndpoints() []string {
	return []string{}
}

func (s *service) Register(ss *grpc.Server) {
	inviteApi.RegisterInviteAPIServer(ss, s)
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}


// New creates a new user share provider svc
func New(m map[string]interface{}, ss *grpc.Server) (rgrpc.Service, error) {

	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	// if driver is empty we default to json
	if c.Driver == "" {
		c.Driver = "json"
	}

	im, err := getInviteManager(c)
	if err != nil {
		return nil, err
	}

	service := &service{
		conf: c,
		im:   im,
	}

	return service, nil
}

func (s *service) GenerateInviteToken(context.Context, *inviteApi.GenerateInviteTokenRequest) (*inviteApi.GenerateInviteTokenResponse, error) {
	panic("implement me")
}

func (s *service) ForwardInvite(context.Context, *inviteApi.ForwardInviteRequest) (*inviteApi.ForwardInviteResponse, error) {
	panic("implement me")
}

func (s *service) AcceptInvite(context.Context, *inviteApi.AcceptInviteRequest) (*inviteApi.AcceptInviteResponse, error) {
	panic("implement me")
}
