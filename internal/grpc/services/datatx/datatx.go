// Copyright 2018-2023 CERN
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

	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	datatx "github.com/cs3org/go-cs3apis/cs3/tx/v1beta1"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	txdriver "github.com/cs3org/reva/pkg/datatx"
	txregistry "github.com/cs3org/reva/pkg/datatx/manager/registry"
	repoRegistry "github.com/cs3org/reva/pkg/datatx/repository/registry"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/plugin"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/cs3org/reva/pkg/utils/cfg"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("datatx", New)
	plugin.RegisterNamespace("grpc.services.datatx.drivers", func(name string, newFunc any) {
		var f txregistry.NewFunc
		utils.Cast(newFunc, &f)
		txregistry.Register(name, f)
	})
}

type config struct {
	// transfer driver
	TxDriver       string                            `mapstructure:"txdriver"`
	TxDrivers      map[string]map[string]interface{} `mapstructure:"txdrivers"`
	StorageDriver  string                            `mapstructure:"storagedriver"`
	StorageDrivers map[string]map[string]interface{} `mapstructure:"storagedrivers"`
	RemoveOnCancel bool                              `mapstructure:"remove_transfer_on_cancel"`
}

type service struct {
	conf          *config
	txManager     txdriver.Manager
	storageDriver txdriver.Repository
}

func (c *config) ApplyDefaults() {
	if c.TxDriver == "" {
		c.TxDriver = "rclone"
	}
}

func (s *service) Register(ss *grpc.Server) {
	datatx.RegisterTxAPIServer(ss, s)
}

func getDatatxManager(ctx context.Context, c *config) (txdriver.Manager, error) {
	if f, ok := txregistry.NewFuncs[c.TxDriver]; ok {
		return f(ctx, c.TxDrivers[c.TxDriver])
	}
	return nil, errtypes.NotFound("datatx service: driver not found: " + c.TxDriver)
}

func getStorageManager(ctx context.Context, c *config) (txdriver.Repository, error) {
	if f, ok := repoRegistry.NewFuncs[c.StorageDriver]; ok {
		return f(ctx, c.StorageDrivers[c.StorageDriver])
	}
	return nil, errtypes.NotFound("datatx service: driver not found: " + c.StorageDriver)
}

// New creates a new datatx svc.
func New(ctx context.Context, m map[string]interface{}) (rgrpc.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	txManager, err := getDatatxManager(ctx, &c)
	if err != nil {
		return nil, err
	}

	storageDriver, err := getStorageManager(ctx, &c)
	if err != nil {
		return nil, err
	}

	service := &service{
		conf:          &c,
		txManager:     txManager,
		storageDriver: storageDriver,
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
	txInfo, startTransferErr := s.txManager.CreateTransfer(ctx, req.SrcTargetUri, req.DestTargetUri)

	// we always save the transfer regardless of start transfer outcome
	// only then, if starting fails, can we try to restart it
	userID := ctxpkg.ContextMustGetUser(ctx).GetId()
	transfer := &txdriver.Transfer{
		TxID:          txInfo.GetId().OpaqueId,
		SrcTargetURI:  req.SrcTargetUri,
		DestTargetURI: req.DestTargetUri,
		ShareID:       req.GetShareId().OpaqueId,
		UserID:        userID,
	}
	if err := s.storageDriver.StoreTransfer(transfer); err != nil {
		err = errors.Wrap(err, "datatx service: error NEW saving transfer share: "+datatx.Status_STATUS_INVALID.String())
		return &datatx.CreateTransferResponse{
			Status: status.NewInvalid(ctx, "error creating transfer"),
		}, err
	}

	// now check start transfer outcome
	if startTransferErr != nil {
		startTransferErr = errors.Wrap(startTransferErr, "datatx service: error starting transfer job")
		return &datatx.CreateTransferResponse{
			Status: status.NewInvalid(ctx, "datatx service: error creating transfer"),
			TxInfo: txInfo,
		}, startTransferErr
	}

	return &datatx.CreateTransferResponse{
		Status: status.NewOK(ctx),
		TxInfo: txInfo,
	}, nil
}

func (s *service) GetTransferStatus(ctx context.Context, req *datatx.GetTransferStatusRequest) (*datatx.GetTransferStatusResponse, error) {
	transfer, err := s.storageDriver.GetTransfer(req.TxId.OpaqueId)
	if err != nil {
		return nil, errtypes.InternalError("datatx service: transfer not found")
	}

	txInfo, err := s.txManager.GetTransferStatus(ctx, req.GetTxId().OpaqueId)
	if err != nil {
		err = errors.Wrap(err, "datatx service: error retrieving transfer status")
		return &datatx.GetTransferStatusResponse{
			Status: status.NewInternal(ctx, err, "datatx service: error getting transfer status"),
			TxInfo: txInfo,
		}, err
	}

	txInfo.ShareId = &ocm.ShareId{OpaqueId: transfer.ShareID}

	return &datatx.GetTransferStatusResponse{
		Status: status.NewOK(ctx),
		TxInfo: txInfo,
	}, nil
}

func (s *service) CancelTransfer(ctx context.Context, req *datatx.CancelTransferRequest) (*datatx.CancelTransferResponse, error) {
	transfer, err := s.storageDriver.GetTransfer(req.TxId.OpaqueId)
	if err != nil {
		return nil, errtypes.InternalError("datatx service: transfer not found")
	}

	transferRemovedMessage := ""
	if s.conf.RemoveOnCancel {
		if err := s.storageDriver.DeleteTransfer(transfer); err != nil {
			err = errors.Wrap(err, "datatx service: error deleting transfer: "+datatx.Status_STATUS_INVALID.String())
			return &datatx.CancelTransferResponse{
				Status: status.NewInvalid(ctx, "error cancelling transfer"),
			}, err
		}
		transferRemovedMessage = "transfer successfully removed"
	}

	txInfo, err := s.txManager.CancelTransfer(ctx, req.GetTxId().OpaqueId)
	if err != nil {
		txInfo.ShareId = &ocm.ShareId{OpaqueId: transfer.ShareID}
		err = errors.Wrapf(err, "(%v) datatx service: error cancelling transfer", transferRemovedMessage)
		return &datatx.CancelTransferResponse{
			Status: status.NewInternal(ctx, err, "error cancelling transfer"),
			TxInfo: txInfo,
		}, err
	}

	txInfo.ShareId = &ocm.ShareId{OpaqueId: transfer.ShareID}

	return &datatx.CancelTransferResponse{
		Status: status.NewOK(ctx),
		TxInfo: txInfo,
	}, nil
}

func (s *service) ListTransfers(ctx context.Context, req *datatx.ListTransfersRequest) (*datatx.ListTransfersResponse, error) {
	userID := ctxpkg.ContextMustGetUser(ctx).GetId()
	transfers, err := s.storageDriver.ListTransfers(req.Filters, userID)
	if err != nil {
		err = errors.Wrap(err, "datatx service: error listing transfers")
		var txInfos []*datatx.TxInfo
		return &datatx.ListTransfersResponse{
			Status:    status.NewInternal(ctx, err, "error listing transfers"),
			Transfers: txInfos,
		}, err
	}

	txInfos := []*datatx.TxInfo{}
	for _, transfer := range transfers {
		txInfos = append(txInfos, &datatx.TxInfo{
			Id:      &datatx.TxId{OpaqueId: transfer.TxID},
			ShareId: &ocm.ShareId{OpaqueId: transfer.ShareID},
		})
	}

	return &datatx.ListTransfersResponse{
		Status:    status.NewOK(ctx),
		Transfers: txInfos,
	}, nil
}

func (s *service) RetryTransfer(ctx context.Context, req *datatx.RetryTransferRequest) (*datatx.RetryTransferResponse, error) {
	transfer, err := s.storageDriver.GetTransfer(req.TxId.OpaqueId)
	if err != nil {
		return nil, errtypes.InternalError("datatx service: transfer not found")
	}

	txInfo, err := s.txManager.RetryTransfer(ctx, req.GetTxId().OpaqueId)
	if err != nil {
		err = errors.Wrap(err, "datatx service: error retrying transfer")
		return &datatx.RetryTransferResponse{
			Status: status.NewInternal(ctx, err, "error retrying transfer"),
			TxInfo: txInfo,
		}, err
	}

	txInfo.ShareId = &ocm.ShareId{OpaqueId: transfer.ShareID}

	return &datatx.RetryTransferResponse{
		Status: status.NewOK(ctx),
		TxInfo: txInfo,
	}, nil
}
