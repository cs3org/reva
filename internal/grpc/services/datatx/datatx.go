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

package datatx

import (
	"context"
	"fmt"
	"strconv"

	storageprovider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	datatx "github.com/cs3org/go-cs3apis/cs3/tx/v1beta1"
	txdriver "github.com/cs3org/reva/pkg/datatx"
	"github.com/cs3org/reva/pkg/datatx/registry"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/user"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("datatx", New)
}

func (c *config) init() {
	// set sane defaults
}

type config struct {
	Driver  string                            `mapstructure:"driver"`
	Drivers map[string]map[string]interface{} `mapstructure:"drivers"`
}

type service struct {
	conf     *config
	txDriver txdriver.TxDriver
}

func (s *service) Register(ss *grpc.Server) {
	datatx.RegisterTxAPIServer(ss, s)
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

func getDriver(c *config) (txdriver.TxDriver, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(c.Drivers[c.Driver])
	}
	return nil, fmt.Errorf("datatx driver not found: %s", c.Driver)
}

// New creates a new datatx svc
func New(m map[string]interface{}, ss *grpc.Server) (rgrpc.Service, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	c.init()

	driver, err := getDriver(c)
	if err != nil {
		return nil, err
	}

	service := &service{
		conf:     c,
		txDriver: driver,
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
	// ----------------------------------------------------------------------------------------
	// TODO implement persistency component and job status check
	// Mechanism:
	// 1. establish a new unique transfer id
	// 2. initiate the transfer: receive the transfer job id from the driver
	// 3. persist the new transfer id together with the job id and (empty) transfer status
	// 4. start a job that periodically checks the driver whether the transfer is still running
	//    until it is not anymore; update the transfer status with each check with the status
	//    returned by the driver
	//
	// Notes:
	// . rclone does NOT fail/error when the src can not be found: TODO ? check if scr exists ?
	//   however: a get status call should give an error anyway
	// ----------------------------------------------------------------------------------------

	// TODO create a new transfer id
	var txID int64

	u, ok := user.ContextGetUser(ctx)
	if !ok {
		return nil, errors.New("could not find user in context")
	}

	srcRemote := u.GetId().GetIdp()
	srcPath := req.GetRef().GetPath()

	// Temp implementation is depending on current rclone version.
	// (token is username for now, it is used like that in the current rclone driver implementation)
	// TODO: re-implement when the connection-string-remote syntax is available in rclone
	// TODO: implement proper token use
	srcToken := u.GetUsername()
	destToken := req.GetGrantee().GetId().GetOpaqueId()

	destRemote := req.GetGrantee().GetId().GetIdp()
	destPath := req.GetRef().GetPath()

	jobID, err := s.txDriver.DoTransfer(srcRemote, srcPath, srcToken, destRemote, destPath, destToken)
	if err != nil {
		return nil, err
	}
	var txStatus datatx.TxInfo_Status
	if jobID <= 0 {
		// no job id but also no error: transfer failed
		txStatus = datatx.TxInfo_STATUS_TRANSFER_FAILED
	}

	if jobID > 0 {
		txStatus = datatx.TxInfo_STATUS_TRANSFER_IN_PROGRESS
	}

	// TODO return actual transfer id; let job id be the transfer id for now
	txID = jobID

	res := &datatx.CreateTransferResponse{
		Status: status.NewOK(ctx),
		TxInfo: &datatx.TxInfo{
			Id: &datatx.TxId{
				OpaqueId: strconv.FormatInt(txID, 10),
			},
			Ref: &storageprovider.Reference{
				Spec: &storageprovider.Reference_Path{
					Path: srcPath,
				},
			},
			Status: txStatus,
		},
	}
	return res, nil
}

func (s *service) GetTransferStatus(ctx context.Context, req *datatx.GetTransferStatusRequest) (*datatx.GetTransferStatusResponse, error) {
	// the transfer id
	txID := req.TxId.OpaqueId

	// TODO find the jobID belonging to the transfer id; let transfer id be the jobID for now
	var jobID int64
	jobID, err := strconv.ParseInt(txID, 10, 64)
	if err != nil {
		return nil, err
	}

	txStatus, err := s.txDriver.GetTransferStatus(jobID)
	if err != nil {
		return nil, err
	}

	res := &datatx.GetTransferStatusResponse{
		Status: status.NewOK(ctx),
		TxInfo: &datatx.TxInfo{
			Id: &datatx.TxId{
				OpaqueId: txID,
			},
			Ref:    nil,
			Status: txStatus,
		},
	}
	return res, nil
}

func (s *service) CancelTransfer(ctx context.Context, req *datatx.CancelTransferRequest) (*datatx.CancelTransferResponse, error) {
	// the transfer id
	txID := req.TxId.OpaqueId

	// TODO find the jobID belonging to the transfer id; let transfer id be the jobID for now
	var jobID int64
	jobID, err := strconv.ParseInt(txID, 10, 64)
	if err != nil {
		return nil, err
	}

	txStatus, err := s.txDriver.CancelTransfer(jobID)
	if err != nil {
		return nil, err
	}

	res := &datatx.CancelTransferResponse{
		Status: status.NewOK(ctx),
		TxInfo: &datatx.TxInfo{
			Id: &datatx.TxId{
				OpaqueId: txID,
			},
			Ref:    nil,
			Status: txStatus,
		},
	}
	return res, nil
}
