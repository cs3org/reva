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

package json

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"sync"
	"time"

	"github.com/cs3org/reva/pkg/datatx/persistency"
	"github.com/cs3org/reva/pkg/datatx/persistency/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

type transferModel struct {
	File      string
	Transfers map[int64]*persistency.Transfer `json:"transfers"`
}

type driver struct {
	config     *config
	sync.Mutex // concurrent access to the file
	model      *transferModel
}

type config struct {
	File string `mapstructure:"file"`
}

func init() {
	registry.Register("json", New)
}

func (c *config) init() error {
	if c.File == "" {
		c.File = "/var/tmp/reva/datatx-transfers.json"
	}
	return nil
}

// New returns a new persistency driver object.
func New(m map[string]interface{}) (persistency.Driver, error) {
	config, err := parseConfig(m)
	if err != nil {
		err = errors.Wrap(err, "error parsing config for json transfer driver")
		return nil, err
	}
	err = config.init()
	if err != nil {
		err = errors.Wrap(err, "error setting config defaults for json transfer driver")
		return nil, err
	}

	// load or create file
	model, err := loadOrCreate(config.File)
	if err != nil {
		err = errors.Wrap(err, "error loading the file containing the transfers")
		return nil, err
	}

	driver := &driver{
		config: config,
		model:  model,
	}

	return driver, nil
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

func loadOrCreate(file string) (*transferModel, error) {

	_, err := os.Stat(file)
	if os.IsNotExist(err) {
		if err := ioutil.WriteFile(file, []byte("{}"), 0700); err != nil {
			err = errors.Wrap(err, "error creating the transfers storage file: "+file)
			return nil, err
		}
	}

	fd, err := os.OpenFile(file, os.O_CREATE, 0644)
	if err != nil {
		err = errors.Wrap(err, "error opening the transfers storage file: "+file)
		return nil, err
	}
	defer fd.Close()

	data, err := ioutil.ReadAll(fd)
	if err != nil {
		err = errors.Wrap(err, "error reading the data")
		return nil, err
	}

	model := &transferModel{}
	if err := json.Unmarshal(data, model); err != nil {
		err = errors.Wrap(err, "error decoding transfers data to json")
		return nil, err
	}

	if model.Transfers == nil {
		model.Transfers = make(map[int64]*persistency.Transfer)
	}

	model.File = file
	return model, nil
}

func (d *driver) SaveTransfer(transfer *persistency.Transfer) (*persistency.Transfer, error) {
	transferID := transfer.TransferID
	if transferID == 0 {
		newTransferID := createTransferID()
		transfer.TransferID = newTransferID
	}
	d.Lock()
	defer d.Unlock()

	d.model.Transfers[transfer.TransferID] = transfer

	data, err := json.Marshal(d.model)
	if err != nil {
		err = errors.Wrap(err, "error encoding transfer data to json")
		return nil, err
	}

	if err := ioutil.WriteFile(d.model.File, data, 0644); err != nil {
		err = errors.Wrap(err, "error writing transfer data to file: "+d.model.File)
		return nil, err
	}

	return transfer, nil
}

func (d *driver) GetTransfer(transferID int64) (*persistency.Transfer, error) {
	transfer, ok := d.model.Transfers[transferID]
	if !ok {
		return nil, errors.New("json: invalid transfer ID")
	}

	return transfer, nil
}

// TODO create an id based on increment ?
func createTransferID() int64 {
	uniqueID := int64(time.Now().Nanosecond())
	return uniqueID
}
