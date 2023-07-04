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

package json

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"sync"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	txv1beta "github.com/cs3org/go-cs3apis/cs3/tx/v1beta1"
	"github.com/cs3org/reva/pkg/datatx"
	"github.com/cs3org/reva/pkg/datatx/repository/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("json", New)
}

type config struct {
	File string `mapstructure:"file"`
}

type mgr struct {
	config     *config
	sync.Mutex // concurrent access to the file
	model      *transfersModel
}

type transfersModel struct {
	Transfers map[string]*datatx.Transfer `json:"transfers"`
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "datatx repository json driver: error decoding configuration")
		return nil, err
	}
	return c, nil
}

func (c *config) init() {
	if c.File == "" {
		c.File = "/var/tmp/reva/datatx-transfers.json"
	}
}

// New returns a json storage driver.
func New(ctx context.Context, m map[string]interface{}) (datatx.Repository, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	c.init()

	model, err := loadOrCreate(c.File)
	if err != nil {
		err = errors.Wrap(err, "datatx repository json driver: error loading the file containing the transfer shares")
		return nil, err
	}

	mgr := &mgr{
		config: c,
		model:  model,
	}

	return mgr, nil
}

func (m *mgr) StoreTransfer(transfer *datatx.Transfer) error {
	m.Lock()
	defer m.Unlock()

	m.model.Transfers[transfer.TxID] = transfer
	err := m.saveModel()
	if err != nil {
		return errors.Wrap(err, "error storing transfer")
	}

	return nil
}

func (m *mgr) DeleteTransfer(transfer *datatx.Transfer) error {
	m.Lock()
	defer m.Unlock()

	delete(m.model.Transfers, transfer.TxID)
	if err := m.saveModel(); err != nil {
		return errors.New("datatx repository json driver: error deleting transfer: error updating model")
	}
	return nil
}

func (m *mgr) GetTransfer(txID string) (*datatx.Transfer, error) {
	m.Lock()
	defer m.Unlock()

	transfer, ok := m.model.Transfers[txID]
	if !ok {
		return nil, errors.New("datatx repository json driver: error getting transfer: not found")
	}
	return transfer, nil
}

func (m *mgr) ListTransfers(filters []*txv1beta.ListTransfersRequest_Filter, userID *userv1beta1.UserId) ([]*datatx.Transfer, error) {
	m.Lock()
	defer m.Unlock()

	var transfers []*datatx.Transfer
	if userID == nil {
		return transfers, errors.New("datatx repository json driver: error listing transfers, userID must be provided")
	}
	for _, transfer := range m.model.Transfers {
		if transfer.UserID.OpaqueId == userID.OpaqueId {
			if len(filters) == 0 {
				transfers = append(transfers, &datatx.Transfer{
					TxID:          transfer.TxID,
					SrcTargetURI:  transfer.SrcTargetURI,
					DestTargetURI: transfer.DestTargetURI,
					ShareID:       transfer.ShareID,
					UserID:        transfer.UserID,
				})
			} else {
				for _, f := range filters {
					if f.Type == txv1beta.ListTransfersRequest_Filter_TYPE_SHARE_ID {
						if f.GetShareId().GetOpaqueId() == transfer.ShareID {
							transfers = append(transfers, &datatx.Transfer{
								TxID:          transfer.TxID,
								SrcTargetURI:  transfer.SrcTargetURI,
								DestTargetURI: transfer.DestTargetURI,
								ShareID:       transfer.ShareID,
								UserID:        transfer.UserID,
							})
						}
					}
				}
			}
		}
	}
	return transfers, nil
}

func (m *mgr) saveModel() error {
	data, err := json.Marshal(m.model)
	if err != nil {
		err = errors.Wrap(err, "datatx repository json driver: error encoding transfer data to json")
		return err
	}

	if err := os.WriteFile(m.config.File, data, 0644); err != nil {
		err = errors.Wrap(err, "datatx repository json driver: error writing transfer data to file: "+m.config.File)
		return err
	}

	return nil
}

func loadOrCreate(file string) (*transfersModel, error) {
	_, err := os.Stat(file)
	if os.IsNotExist(err) {
		if err := os.WriteFile(file, []byte("{}"), 0700); err != nil {
			err = errors.Wrap(err, "datatx repository json driver: error creating the datatx shares storage file: "+file)
			return nil, err
		}
	}

	fd, err := os.OpenFile(file, os.O_CREATE, 0644)
	if err != nil {
		err = errors.Wrap(err, "datatx repository json driver: error opening the datatx shares storage file: "+file)
		return nil, err
	}
	defer fd.Close()

	data, err := io.ReadAll(fd)
	if err != nil {
		err = errors.Wrap(err, "datatx repository json driver: error reading the data")
		return nil, err
	}

	model := &transfersModel{}
	if err := json.Unmarshal(data, model); err != nil {
		err = errors.Wrap(err, "datatx repository json driver: error decoding datatx shares data to json")
		return nil, err
	}

	if model.Transfers == nil {
		model.Transfers = make(map[string]*datatx.Transfer)
	}

	return model, nil
}
