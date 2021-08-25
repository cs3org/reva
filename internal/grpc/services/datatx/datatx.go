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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"sync"

	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	datatx "github.com/cs3org/go-cs3apis/cs3/tx/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	txdriver "github.com/cs3org/reva/pkg/datatx"
	txregistry "github.com/cs3org/reva/pkg/datatx/manager/registry"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/token"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("datatx", New)
}

type config struct {
	// transfer driver
	TxDriver  string                            `mapstructure:"txdriver"`
	TxDrivers map[string]map[string]interface{} `mapstructure:"txdrivers"`
	// storage driver to persist share/transfer relation
	StorageDriver       string                            `mapstructure:"storage_driver"`
	StorageDrivers      map[string]map[string]interface{} `mapstructure:"storage_drivers"`
	TxSharesFile        string                            `mapstructure:"tx_shares_file"`
	DataTransfersFolder string                            `mapstructure:"data_transfers_folder"`
}

type service struct {
	conf          *config
	txManager     txdriver.Manager
	txShareDriver *txShareDriver
}

type txShareDriver struct {
	sync.Mutex // concurrent access to the file
	model      *txShareModel
}
type txShareModel struct {
	File     string
	TxShares map[string]*txShare `json:"shares"`
}

type txShare struct {
	TxID      string
	TargetUri string
	Opaque    *types.Opaque `json:"opaque"`
}

type webdavEndpoint struct {
	filePath       string
	endpoint       string
	endpointScheme string
	token          string
}

func (c *config) init() {
	if c.TxDriver == "" {
		c.TxDriver = "rclone"
	}
	if c.DataTransfersFolder == "" {
		c.DataTransfersFolder = "DataTransfers"
	}
}

func (s *service) Register(ss *grpc.Server) {
	datatx.RegisterTxAPIServer(ss, s)
}

func getDatatxManager(c *config) (txdriver.Manager, error) {
	if f, ok := txregistry.NewFuncs[c.TxDriver]; ok {
		return f(c.TxDrivers[c.TxDriver])
	}
	return nil, errtypes.NotFound("datatx service: driver not found: " + c.TxDriver)
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "datatx service: error decoding conf")
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

	txManager, err := getDatatxManager(c)
	if err != nil {
		return nil, err
	}

	model, err := loadOrCreate(c.TxSharesFile)
	if err != nil {
		err = errors.Wrap(err, "datatx service: error loading the file containing the transfer shares")
		return nil, err
	}
	txShareDriver := &txShareDriver{
		model: model,
	}

	service := &service{
		conf:          c,
		txManager:     txManager,
		txShareDriver: txShareDriver,
	}

	return service, nil
}

func (s *service) Close() error {
	return nil
}

func (s *service) UnprotectedEndpoints() []string {
	return []string{}
}

func (s *service) PullTransfer(ctx context.Context, req *datatx.PullTransferRequest) (*datatx.PullTransferResponse, error) {
	fmt.Printf("PullTransfer reached: req: %v\n", req)
	var srcRemote string
	var srcPath string
	var srcToken string
	var dstRemote string
	var dstPath string
	var dstToken string
	srcRemote = ""
	srcPath = ""
	srcToken = ""
	dstRemote = ""
	dstPath = ""
	dstToken = ""

	ep, err := s.extractEndpointInfo(ctx, req.GetTargetUri())
	if err != nil {
		return nil, err
	}
	srcRemote = fmt.Sprintf("%s://%s", ep.endpointScheme, ep.endpoint)
	srcPath = ep.filePath
	srcToken = ep.token
	// destination(grantee) webdav endpoint
	// user := user.ContextMustGetUser(ctx) -> user.Id.Idp
	endpoint, ok := req.Opaque.Map["endpoint"]
	if !ok {
		return nil, errtypes.NotSupported("endpoint not defined")
	}
	dstRemote = string(endpoint.Value)
	// // home dir prefix must be removed from the path
	// dstPath = path.Join(s.conf.DataTransfersFolder, strings.TrimPrefix(ep.filePath, "/home"))
	dstPath = path.Join(s.conf.DataTransfersFolder, ep.filePath)
	dstToken = token.ContextMustGetToken(ctx)

	txInfo, err := s.txManager.StartTransfer(ctx, srcRemote, srcPath, srcToken, dstRemote, dstPath, dstToken)
	if err != nil {
		err = errors.Wrap(err, "datatx service: error starting transfer job")
		return &datatx.PullTransferResponse{
			Status: status.NewInvalid(ctx, "datatx service: error pulling transfer"),
			TxInfo: txInfo,
		}, err
	}
	fmt.Printf("err: %v\n", err)
	fmt.Printf("txInfo Status: %v\n", txInfo.Status)
	fmt.Printf("txInfo TxID: %v\n", txInfo.GetId().OpaqueId)
	fmt.Printf("txInfo Ctime: %v\n", txInfo.GetCtime())

	txShare := &txShare{
		TxID:      txInfo.GetId().OpaqueId,
		TargetUri: req.TargetUri,
		Opaque:    req.Opaque,
	}

	s.txShareDriver.Lock()
	defer s.txShareDriver.Unlock()

	s.txShareDriver.model.TxShares[txInfo.GetId().OpaqueId] = txShare

	if err := s.txShareDriver.model.saveTxShare(); err != nil {
		err = errors.Wrap(err, "datatx service: error saving transfer share: "+datatx.Status_STATUS_INVALID.String())
		return &datatx.PullTransferResponse{
			Status: status.NewInvalid(ctx, "error pulling transfer"),
		}, err
	}

	return &datatx.PullTransferResponse{
		Status: status.NewOK(ctx),
		TxInfo: txInfo,
	}, err
}

func (s *service) GetTransferStatus(ctx context.Context, req *datatx.GetTransferStatusRequest) (*datatx.GetTransferStatusResponse, error) {
	txShare, ok := s.txShareDriver.model.TxShares[req.GetTxId().GetOpaqueId()]
	if !ok {
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

	txInfo.ShareId = &ocm.ShareId{OpaqueId: string(txShare.Opaque.Map["shareId"].Value)}

	return &datatx.GetTransferStatusResponse{
		Status: status.NewOK(ctx),
		TxInfo: txInfo,
	}, nil
}

func (s *service) CancelTransfer(ctx context.Context, req *datatx.CancelTransferRequest) (*datatx.CancelTransferResponse, error) {
	txShare, ok := s.txShareDriver.model.TxShares[req.GetTxId().GetOpaqueId()]
	if !ok {
		return nil, errtypes.InternalError("datatx service: transfer not found")
	}

	txInfo, err := s.txManager.CancelTransfer(ctx, req.GetTxId().OpaqueId)
	if err != nil {
		txInfo.ShareId = &ocm.ShareId{OpaqueId: string(txShare.Opaque.Map["shareId"].Value)}
		txInfo.Status = datatx.Status_STATUS_TRANSFER_CANCELLED
		return &datatx.CancelTransferResponse{
			Status: status.NewOK(ctx),
			TxInfo: txInfo,
		}, nil

		// err = errors.Wrap(err, "datatx service: error cancelling transfer")
		// return &datatx.CancelTransferResponse{
		// 	Status: status.NewInternal(ctx, err, "error cancelling transfer"),
		// 	TxInfo: txInfo,
		// }, err
	}

	txInfo.ShareId = &ocm.ShareId{OpaqueId: string(txShare.Opaque.Map["shareId"].Value)}

	return &datatx.CancelTransferResponse{
		Status: status.NewOK(ctx),
		TxInfo: txInfo,
	}, nil
}

func (s *service) ListTransfers(ctx context.Context, req *datatx.ListTransfersRequest) (*datatx.ListTransfersResponse, error) {
	filters := req.Filters
	var txInfos []*datatx.TxInfo
	for _, txShare := range s.txShareDriver.model.TxShares {
		if len(filters) == 0 {
			txInfos = append(txInfos, &datatx.TxInfo{
				Id:      &datatx.TxId{OpaqueId: txShare.TxID},
				ShareId: &ocm.ShareId{OpaqueId: string(txShare.Opaque.Map["shareId"].Value)},
			})
		} else {
			for _, f := range filters {
				if f.Type == datatx.ListTransfersRequest_Filter_TYPE_SHARE_ID {
					if f.GetShareId().GetOpaqueId() == string(txShare.Opaque.Map["shareId"].Value) {
						txInfos = append(txInfos, &datatx.TxInfo{
							Id:      &datatx.TxId{OpaqueId: txShare.TxID},
							ShareId: &ocm.ShareId{OpaqueId: string(txShare.Opaque.Map["shareId"].Value)},
						})
					}
				}
			}
		}
	}

	return &datatx.ListTransfersResponse{
		Status:    status.NewOK(ctx),
		Transfers: txInfos,
	}, nil
}

func (s *service) RetryTransfer(ctx context.Context, req *datatx.RetryTransferRequest) (*datatx.RetryTransferResponse, error) {
	txShare, ok := s.txShareDriver.model.TxShares[req.GetTxId().GetOpaqueId()]
	if !ok {
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

	txInfo.ShareId = &ocm.ShareId{OpaqueId: string(txShare.Opaque.Map["shareId"].Value)}

	return &datatx.RetryTransferResponse{
		Status: status.NewOK(ctx),
		TxInfo: txInfo,
	}, nil
}

func (s *service) extractEndpointInfo(ctx context.Context, targetURL string) (*webdavEndpoint, error) {
	if targetURL == "" {
		return nil, errtypes.BadRequest("datatx service: ref target is an empty uri")
	}

	uri, err := url.Parse(targetURL)
	if err != nil {
		return nil, errors.Wrap(err, "datatx service: error parsing target uri: "+targetURL)
	}
	if uri.Scheme != "datatx" {
		return nil, errtypes.NotSupported("datatx service: ref target does not have the datatx scheme")
	}

	m, err := url.ParseQuery(uri.RawQuery)
	if err != nil {
		return nil, errors.Wrap(err, "datatx service: error parsing target resource name")
	}

	return &webdavEndpoint{
		filePath:       m["name"][0],
		endpoint:       uri.Host + uri.Path,
		endpointScheme: m["endpointscheme"][0],
		token:          uri.User.String(),
	}, nil
}

func loadOrCreate(file string) (*txShareModel, error) {
	_, err := os.Stat(file)
	if os.IsNotExist(err) {
		if err := ioutil.WriteFile(file, []byte("{}"), 0700); err != nil {
			err = errors.Wrap(err, "datatx service: error creating the transfer shares storage file: "+file)
			return nil, err
		}
	}

	fd, err := os.OpenFile(file, os.O_CREATE, 0644)
	if err != nil {
		err = errors.Wrap(err, "datatx service: error opening the transfer shares storage file: "+file)
		return nil, err
	}
	defer fd.Close()

	data, err := ioutil.ReadAll(fd)
	if err != nil {
		err = errors.Wrap(err, "datatx service: error reading the data")
		return nil, err
	}

	model := &txShareModel{}
	if err := json.Unmarshal(data, model); err != nil {
		err = errors.Wrap(err, "datatx service: error decoding transfer shares data to json")
		return nil, err
	}

	if model.TxShares == nil {
		model.TxShares = make(map[string]*txShare)
	}

	model.File = file
	return model, nil
}

func (m *txShareModel) saveTxShare() error {
	data, err := json.Marshal(m)
	if err != nil {
		err = errors.Wrap(err, "datatx service: error encoding transfer share data to json")
		return err
	}

	if err := ioutil.WriteFile(m.File, data, 0644); err != nil {
		err = errors.Wrap(err, "datatx service: error writing transfer share data to file: "+m.File)
		return err
	}

	return nil
}
