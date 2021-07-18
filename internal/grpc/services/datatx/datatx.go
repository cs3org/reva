// Copyright 2018-2021 CERN
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

package datatx

import (
	"context"

	datatx "github.com/cs3org/go-cs3apis/cs3/tx/v1beta1"
	driver "github.com/cs3org/reva/pkg/datatx"
	"github.com/cs3org/reva/pkg/datatx/manager/registry"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/storage"
	fsreg "github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("datatx", New)
}

type config struct {
	Driver    string                            `mapstructure:"driver"`
	Drivers   map[string]map[string]interface{} `mapstructure:"drivers"`
	FSDriver  string                            `mapstructure:"fsdriver"`
	FSDrivers map[string]map[string]interface{} `mapstructure:"fsdrivers"`
}

type service struct {
	conf    *config
	datatx  driver.Manager
	storage storage.FS
}

func (c *config) init() {
	if c.Driver == "" {
		c.Driver = "rclone"
	}
	if c.FSDriver == "" {
		c.FSDriver = "localhome"
	}
}

func (s *service) Register(ss *grpc.Server) {
	datatx.RegisterTxAPIServer(ss, s)
}

func getDatatxManager(c *config) (driver.Manager, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(c.Drivers[c.Driver])
	}
	return nil, errtypes.NotFound("driver not found: " + c.Driver)
}

func getFS(c *config) (storage.FS, error) {
	if f, ok := fsreg.NewFuncs[c.FSDriver]; ok {
		return f(c.Drivers[c.FSDriver])
	}
	return nil, errtypes.NotFound("driver not found: " + c.FSDriver)
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

// New creates a new datatx svc
func New(m map[string]interface{}, ss *grpc.Server) (rgrpc.Service, error) {

	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	c.init()

	datatx, err := getDatatxManager(c)
	if err != nil {
		return nil, err
	}

	fs, err := getFS(c)
	if err != nil {
		return nil, err
	}

	service := &service{
		conf:    c,
		datatx:  datatx,
		storage: fs,
	}

	return service, nil
}

func (s *service) Close() error {
	return nil
}

func (s *service) UnprotectedEndpoints() []string {
	return []string{}
}

func (s *service) CreateTransfer(ctx context.Context, req *datatx.CreateTransferRequest) (*datatx.CreateTransferResponse, error) {
	return &datatx.CreateTransferResponse{
		Status: status.NewUnimplemented(ctx, errtypes.NotSupported("CreateTransfer not implemented"), "CreateTransfer not implemented"),
	}, nil
}

func (s *service) GetTransferStatus(ctx context.Context, req *datatx.GetTransferStatusRequest) (*datatx.GetTransferStatusResponse, error) {
	txStatus, err := s.datatx.GetTransferStatus(req.TxId.OpaqueId)
	if err != nil {
		return &datatx.GetTransferStatusResponse{
			Status: status.NewInternal(ctx, err, "error requesting transfer status"),
		}, nil
	}
	return &datatx.GetTransferStatusResponse{
		Status: status.NewOK(ctx),
		TxInfo: &datatx.TxInfo{
			Id:     req.TxId,
			Status: txStatus,
		},
	}, nil
}

func (s *service) CancelTransfer(ctx context.Context, req *datatx.CancelTransferRequest) (*datatx.CancelTransferResponse, error) {
	txStatus, err := s.datatx.CancelTransfer(req.TxId.OpaqueId)
	if err != nil {
		return &datatx.CancelTransferResponse{
			Status: status.NewInternal(ctx, err, "error cancelling transfer"),
		}, nil
	}
	return &datatx.CancelTransferResponse{
		Status: status.NewOK(ctx),
		TxInfo: &datatx.TxInfo{
			Id:     req.TxId,
			Status: txStatus,
		},
	}, nil
}
