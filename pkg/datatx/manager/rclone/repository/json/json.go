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

	"github.com/cs3org/reva/pkg/datatx/manager/rclone/repository"
	"github.com/cs3org/reva/pkg/datatx/manager/rclone/repository/registry"
	"github.com/cs3org/reva/pkg/utils/cfg"
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
	model      *rcloneJobsModel
}

type rcloneJobsModel struct {
	RcloneJobs map[string]*repository.Job `json:"rcloneJobs"`
}

func (c *config) ApplyDefaults() {
	if c.File == "" {
		c.File = "/var/tmp/reva/transfer-jobs.json"
	}
}

// New returns a json storage driver.
func New(ctx context.Context, m map[string]interface{}) (repository.Repository, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	model, err := loadOrCreate(c.File)
	if err != nil {
		err = errors.Wrap(err, "rclone repository json driver: error loading the file containing the transfer shares")
		return nil, err
	}

	mgr := &mgr{
		config: &c,
		model:  model,
	}

	return mgr, nil
}

func (m *mgr) StoreJob(job *repository.Job) error {
	m.Lock()
	defer m.Unlock()

	m.model.RcloneJobs[job.TransferID] = job
	err := m.saveModel()
	if err != nil {
		return errors.Wrap(err, "error storing jobs")
	}

	return nil
}

func (m *mgr) GetJob(transferID string) (*repository.Job, error) {
	m.Lock()
	defer m.Unlock()

	job, ok := m.model.RcloneJobs[transferID]
	if !ok {
		return nil, errors.New("rclone repository json driver: error getting job: not found")
	}
	return job, nil
}

func (m *mgr) DeleteJob(job *repository.Job) error {
	m.Lock()
	defer m.Unlock()

	delete(m.model.RcloneJobs, job.TransferID)
	if err := m.saveModel(); err != nil {
		return errors.New("rclone repository json driver: error deleting job: error updating model")
	}
	return nil
}

func (m *mgr) saveModel() error {
	data, err := json.Marshal(m.model)
	if err != nil {
		err = errors.Wrap(err, "rclone repository json driver: error encoding job data to json")
		return err
	}

	if err := os.WriteFile(m.config.File, data, 0644); err != nil {
		err = errors.Wrap(err, "rclone repository json driver: error writing job data to file: "+m.config.File)
		return err
	}

	return nil
}

func loadOrCreate(file string) (*rcloneJobsModel, error) {
	_, err := os.Stat(file)
	if os.IsNotExist(err) {
		if err := os.WriteFile(file, []byte("{}"), 0700); err != nil {
			err = errors.Wrap(err, "rclone repository json driver: error creating the jobs storage file: "+file)
			return nil, err
		}
	}

	fd, err := os.OpenFile(file, os.O_CREATE, 0644)
	if err != nil {
		err = errors.Wrap(err, "rclone repository json driver: error opening the jobs storage file: "+file)
		return nil, err
	}
	defer fd.Close()

	data, err := io.ReadAll(fd)
	if err != nil {
		err = errors.Wrap(err, "rclone repository json driver: error reading the data")
		return nil, err
	}

	model := &rcloneJobsModel{}
	if err := json.Unmarshal(data, model); err != nil {
		err = errors.Wrap(err, "rclone repository json driver: error decoding jobs data to json")
		return nil, err
	}

	if model.RcloneJobs == nil {
		model.RcloneJobs = make(map[string]*repository.Job)
	}

	return model, nil
}
