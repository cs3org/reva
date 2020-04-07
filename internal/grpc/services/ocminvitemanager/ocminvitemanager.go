// Copyright 2018-2020 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package ocminvitemanager

import (
	"context"
	"fmt"
	"log"

	invitepb "github.com/cs3org/go-cs3apis/cs3/invite/v1beta1"
	"github.com/cs3org/reva/pkg/ocm/invite"
	"github.com/cs3org/reva/pkg/ocm/invite/manager/registry"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func init() {
	log.Println("################## init  invite########")
	rgrpc.Register("ocminvitemanager", New)
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
	log.Println("################## init  getinviteM########")
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		log.Println("################## init  OK########")
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
	invitepb.RegisterInviteAPIServer(ss, s)
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

// New creates a new OCM invite manager svc
func New(m map[string]interface{}, ss *grpc.Server) (rgrpc.Service, error) {

	log.Println("################## init  1########")
	c, err := parseConfig(m)
	if err != nil {

		log.Println("################## init  2########")
		return nil, err
	}

	// if driver is empty we default to json
	if c.Driver == "" {
		c.Driver = "json"
	}

	log.Println("################## init  3########")
	im, err := getInviteManager(c)
	if err != nil {

		log.Println("################## init  4########")
		return nil, err
	}

	service := &service{
		conf: c,
		im:   im,
	}

	log.Println("################## init  5########")
	return service, nil
}

func (s *service) GenerateInviteToken(ctx context.Context, req *invitepb.GenerateInviteTokenRequest) (*invitepb.GenerateInviteTokenResponse, error) {
	token, err := s.im.GenerateToken(ctx)
	if err != nil {
		return &invitepb.GenerateInviteTokenResponse{
			Status: status.NewInternal(ctx, err, "error generating invite token"),
		}, nil
	}

	return &invitepb.GenerateInviteTokenResponse{
		Status:      status.NewOK(ctx),
		InviteToken: token,
	}, nil
}

<<<<<<< HEAD
func (s *service) ForwardInvite(ctx context.Context, req *invitepb.ForwardInviteRequest) (*invitepb.ForwardInviteResponse, error) {
	err := s.im.ForwardInvite(ctx, req.InviteToken, req.OriginSystemProvider)
	if err != nil {
		return &invitepb.ForwardInviteResponse{
			Status: status.NewInternal(ctx, err, "error forwarding invite"),
		}, nil
	}

	return &invitepb.ForwardInviteResponse{
		Status: status.NewOK(ctx),
	}, nil
=======
func (s *service) ForwardInvite(ctx context.Context, req *ocminvite.ForwardInviteRequest) (*ocminvite.ForwardInviteResponse, error) {
	//Not yet implemented
	log := appctx.GetLogger(ctx)
	log.Info().Msg("grpc/ocminvitemanager/ForwardInviteManager**********")

	token := req.InviteToken
	originSystemProvider := req.OriginSystemProvider
	err := s.im.ForwardInvite(ctx, token, originSystemProvider)
	if err != nil {
		return &ocminvite.ForwardInviteResponse{
			Status: status.NewInternal(ctx, err, "error creating share"),
		}, nil
	}

	res := &ocminvite.ForwardInviteResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
>>>>>>> WIP 2
}

func (s *service) AcceptInvite(ctx context.Context, req *invitepb.AcceptInviteRequest) (*invitepb.AcceptInviteResponse, error) {
	err := s.im.AcceptInvite(ctx, req.InviteToken, req.UserId)
	if err != nil {
		return &invitepb.AcceptInviteResponse{
			Status: status.NewInternal(ctx, err, "error accepting invite"),
		}, nil
	}

	return &invitepb.AcceptInviteResponse{
		Status: status.NewOK(ctx),
	}, nil
}
