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
	"time"

	storageprovider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	datatx "github.com/cs3org/go-cs3apis/cs3/tx/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	txdriver "github.com/cs3org/reva/pkg/datatx"
	"github.com/cs3org/reva/pkg/datatx/manager/registry"
	txpersistency "github.com/cs3org/reva/pkg/datatx/persistency"
	txpersistencyregistry "github.com/cs3org/reva/pkg/datatx/persistency/registry"
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
	if c.Driver == "" {
		c.Driver = "rclone"
	}
	if c.JobStatusCheckInterval == 0 {
		c.JobStatusCheckInterval = 2000
	}
}

type config struct {
	Driver                 string                            `mapstructure:"driver"`
	Drivers                map[string]map[string]interface{} `mapstructure:"drivers"`
	PersistencyDriver      string                            `mapstructure:"persistency_driver"`
	PersistencyDrivers     map[string]map[string]interface{} `mapstructure:"persistency_drivers"`
	JobStatusCheckInterval int                               `mapstructure:"job_status_check_interval"`
	JobTimeout             int                               `mapstructure:"job_timeout"`
}

type service struct {
	conf                *config
	txDriver            txdriver.TxDriver
	txPersistencyDriver txpersistency.Driver
}

// txEndStatuses final statuses that cannot be changed anymore
var txEndStatuses = map[string]int32{
	"STATUS_INVALID":                0,
	"STATUS_DESTINATION_NOT_FOUND":  1,
	"STATUS_TRANSFER_COMPLETE":      6,
	"STATUS_TRANSFER_FAILED":        7,
	"STATUS_TRANSFER_CANCELLED":     8,
	"STATUS_TRANSFER_CANCEL_FAILED": 9,
	"STATUS_TRANSFER_EXPIRED":       10,
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

func getPersistencyDriver(c *config) (txpersistency.Driver, error) {
	if f, ok := txpersistencyregistry.NewFuncs[c.PersistencyDriver]; ok {
		return f(c.PersistencyDrivers[c.PersistencyDriver])
	}
	return nil, fmt.Errorf("datatx persistency driver not found: %s", c.PersistencyDriver)
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

	persistencyDriver, err := getPersistencyDriver(c)
	if err != nil {
		return nil, err
	}

	service := &service{
		conf:                c,
		txDriver:            driver,
		txPersistencyDriver: persistencyDriver,
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
	// TODO implement persistency component and status check job
	// Mechanism:
	// 1. establish a new unique transfer id
	// 2. persist the new transfer id together with the job id and (empty) transfer status
	// 3. do OCM core share request (datatx protocol share type) towards the destination and receive a token
	// 4. initiate the transfer: receive the transfer job id from the driver
	// 5. start a job that periodically checks the status of the persisted transfer:
	//    . case OCM share is accepted: start transfer, update transfer status accordingly
	//    . case transfer is finished: update transfer status accordingly
	//
	// Notes:
	// . rclone does NOT fail/error when the src can not be found: TODO ? check if scr exists ?
	//   however: a get status call should give an error anyway
	// ----------------------------------------------------------------------------------------
	log := appctx.GetLogger(ctx)

	u, ok := user.ContextGetUser(ctx)
	if !ok {
		return nil, errors.New("could not find user in context")
	}

	srcRemote := u.GetId().GetIdp()
	srcPath := req.GetRef().GetPath()

	// Temp implementation is depending on current rclone version.
	// (token is username for now, it is used like that in the current rclone driver implementation)
	// TODO: re-implement once the connection-string-remote syntax is available in rclone
	// TODO: implement proper token use

	srcToken := u.GetUsername()
	destToken := req.GetGrantee().GetId().GetOpaqueId()

	destRemote := req.GetGrantee().GetId().GetIdp()
	destPath := req.GetRef().GetPath()

	transfer := &txpersistency.Transfer{
		TransferStatus: datatx.TxInfo_Status_value[datatx.TxInfo_STATUS_TRANSFER_NEW.String()],
		SrcRemote:      srcRemote,
		SrcPath:        srcPath,
		DestRemote:     destRemote,
		DestPath:       destPath,
	}

	savedTransfer, err := s.txPersistencyDriver.SaveTransfer(transfer)
	if err != nil {
		return nil, err
	}

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

	savedTransfer.JobID = jobID
	savedTransfer.TransferStatus = datatx.TxInfo_Status_value[txStatus.String()]

	// update the new transfer with the job ID and status
	updatedTransfer, err := s.txPersistencyDriver.SaveTransfer(savedTransfer)
	if err != nil {
		return nil, err
	}

	transferID := updatedTransfer.TransferID

	// Periodically checks the status of the transfer until it is finished,
	// by consulting the tx driver for the current status and update the transfer accordingly
	//
	go func() {
		// runs as long as no end state or time out has been reached
		startTimeMs := time.Now().Nanosecond() / 1000
		timeout := s.conf.JobTimeout
		// fmt.Printf("start time: %v (timeout: %v)\n", startTimeMs, timeout)
		for {
			currentTimeMs := time.Now().Nanosecond() / 1000
			timePastMs := currentTimeMs - startTimeMs
			// fmt.Printf("current time: %v (time past: %v)\n", currentTimeMs, timePastMs)
			if timePastMs > timeout {
				// set status to EXPIRED
				fmt.Printf("JOB TIMED OUT\n")
				_, err := s.expireJob(transferID)
				if err != nil {
					log.Err(err).Msg("could not expire transfer job")
				}
				break
			}

			// get and verify the persisted transfer status first;
			// a transfer end status may have been reached
			transfer, err := s.txPersistencyDriver.GetTransfer(transferID)
			if err != nil {
				log.Err(err).Msg("error retrieving transfer job status")
				break
			}
			currentTransferStatus := transfer.TransferStatus
			currentTxStatus := datatx.TxInfo_Status(currentTransferStatus)
			_, endStatusFound := txEndStatuses[currentTxStatus.String()]
			if endStatusFound {
				// we're done already
				fmt.Printf("TRANSFER FINISHED: %v\n", currentTxStatus)
				break
			}

			// get the latest transfer status from the txdriver
			txStatus, err := s.txDriver.GetTransferStatus(jobID)
			if err != nil {
				log.Debug().Err(err).Msg("error retrieving transfer job status. Status returned: " + txStatus.String())
			}
			latestTransferStatus := datatx.TxInfo_Status_value[txStatus.String()]

			// update (persist) transfer status if it has changed
			if currentTransferStatus != latestTransferStatus {
				err := s.updateTransferStatus(transferID, latestTransferStatus)
				if err != nil {
					log.Err(err).Msg("could not update transfer status")
				}
			}

			fmt.Printf("... checking\n")
			<-time.After(time.Millisecond * time.Duration(s.conf.JobStatusCheckInterval))
		}
	}()

	res := &datatx.CreateTransferResponse{
		Status: status.NewOK(ctx),
		TxInfo: &datatx.TxInfo{
			Id: &datatx.TxId{
				OpaqueId: strconv.FormatInt(transferID, 10),
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

func (s *service) expireJob(transferID int64) (datatx.TxInfo_Status, error) {
	txStatus := datatx.TxInfo_STATUS_TRANSFER_EXPIRED
	transfer, err := s.txPersistencyDriver.GetTransfer(transferID)
	if err != nil {
		return datatx.TxInfo_STATUS_INVALID, err
	}
	transfer.TransferStatus = datatx.TxInfo_Status_value[txStatus.String()]

	expiredTransfer, err := s.txPersistencyDriver.SaveTransfer(transfer)
	if err != nil {
		return datatx.TxInfo_STATUS_INVALID, err
	}

	// TODO find the jobID belonging to the transfer id; let transfer id be the jobID for now
	// jobID, err := strconv.ParseInt(txID, 10, 64)

	fmt.Printf("job expired: %v\n", datatx.TxInfo_Status(expiredTransfer.TransferStatus))
	return datatx.TxInfo_Status(expiredTransfer.TransferStatus), nil
}

// simple transfer status update
func (s *service) updateTransferStatus(transferID int64, transferStatus int32) error {
	transfer, err := s.txPersistencyDriver.GetTransfer(transferID)
	if err != nil {
		return err
	}
	transfer.TransferStatus = transferStatus

	_, err = s.txPersistencyDriver.SaveTransfer(transfer)
	if err != nil {
		return err
	}

	fmt.Printf("transfer status updated: %v\n", datatx.TxInfo_Status(transferStatus))
	return nil
}

func (s *service) GetTransferStatus(ctx context.Context, req *datatx.GetTransferStatusRequest) (*datatx.GetTransferStatusResponse, error) {
	transferID, err := strconv.ParseInt(req.GetTxId().GetOpaqueId(), 10, 64)
	if err != nil {
		return nil, err
	}

	txStatus, err := s.getTransferJobStatus(transferID)
	if err != nil {
		return nil, err
	}

	res := &datatx.GetTransferStatusResponse{
		Status: status.NewOK(ctx),
		TxInfo: &datatx.TxInfo{
			Id: &datatx.TxId{
				OpaqueId: strconv.FormatInt(transferID, 10),
			},
			Ref:    nil,
			Status: txStatus,
		},
	}
	return res, nil
}

func (s *service) CancelTransfer(ctx context.Context, req *datatx.CancelTransferRequest) (*datatx.CancelTransferResponse, error) {
	transferID, err := strconv.ParseInt(req.GetTxId().GetOpaqueId(), 10, 64)
	if err != nil {
		return nil, err
	}
	transfer, err := s.txPersistencyDriver.GetTransfer(transferID)
	if err != nil {
		return nil, err
	}
	jobID := transfer.JobID

	currentTxStatus, err := s.getTransferJobStatus(transferID)
	if err != nil {
		return nil, err
	}

	// cannot cancel final statuses
	endStatus, endStatusFound := txEndStatuses[currentTxStatus.String()]
	if endStatusFound {
		fmt.Printf("an end status has already been reached: %v. Cannot cancel this.\n", endStatus)
		res := &datatx.CancelTransferResponse{
			Status: status.NewOK(ctx),
			TxInfo: &datatx.TxInfo{
				Id: &datatx.TxId{
					OpaqueId: req.GetTxId().GetOpaqueId(),
				},
				Ref:    nil,
				Status: datatx.TxInfo_Status(endStatus),
			},
		}
		return res, nil
	}

	txStatus, err := s.txDriver.CancelTransfer(jobID)
	if err != nil {
		return nil, err
	}

	// update the canceled transfer
	transfer.TransferStatus = datatx.TxInfo_Status_value[txStatus.String()]
	updatedTransfer, err := s.txPersistencyDriver.SaveTransfer(transfer)
	if err != nil {
		return nil, err
	}

	res := &datatx.CancelTransferResponse{
		Status: status.NewOK(ctx),
		TxInfo: &datatx.TxInfo{
			Id: &datatx.TxId{
				OpaqueId: strconv.FormatInt(updatedTransfer.TransferID, 10),
			},
			Ref:    nil,
			Status: txStatus,
		},
	}
	return res, nil
}

func (s *service) getTransferJobStatus(transferID int64) (datatx.TxInfo_Status, error) {
	fmt.Printf("checking transfer job status: %v\n", transferID)

	transfer, err := s.txPersistencyDriver.GetTransfer(transferID)
	if err != nil {
		return datatx.TxInfo_STATUS_INVALID, err
	}
	txStatus := datatx.TxInfo_Status(transfer.TransferStatus)

	return txStatus, nil
}
