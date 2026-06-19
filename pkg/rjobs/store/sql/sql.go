// Copyright 2018-2026 CERN
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

// Package sql implements the rjobs.StatusStore on top of a relational
// database via GORM. It keeps one row per run in the job_runs table, updated
// as the run moves through its lifecycle, and serves it back by run id.
package sql

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/rjobs"
	"github.com/cs3org/reva/v3/pkg/rjobs/store/sql/model"
	"github.com/pkg/errors"
	"gorm.io/datatypes"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type store struct {
	db *gorm.DB
}

// New opens the database, migrates the schema and returns a status store.
func New(ctx context.Context, c config.Database) (rjobs.StatusStore, error) {
	db, err := openDB(c)
	if err != nil {
		return nil, errors.Wrap(err, "rjobs sql: opening database failed")
	}
	if err := db.AutoMigrate(&model.Run{}); err != nil {
		return nil, errors.Wrap(err, "rjobs sql: migrating schema failed")
	}
	return &store{db: db}, nil
}

func openDB(c config.Database) (*gorm.DB, error) {
	gormCfg := &gorm.Config{}
	switch c.Engine {
	case "sqlite":
		return gorm.Open(sqlite.Open(c.DBName), gormCfg)
	default: // mysql
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true", c.DBUsername, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
		return gorm.Open(mysql.Open(dsn), gormCfg)
	}
}

func (s *store) Put(ctx context.Context, st rjobs.Status) error {
	row, err := toModel(st)
	if err != nil {
		return err
	}
	// upsert on the run id: the row is created on enqueue and updated on every
	// later transition.
	res := s.db.WithContext(ctx).Save(row)
	if res.Error != nil {
		return errors.Wrap(res.Error, "rjobs sql: storing status failed")
	}
	return nil
}

func (s *store) Get(ctx context.Context, id rjobs.RunID) (rjobs.Status, error) {
	var row model.Run
	res := s.db.WithContext(ctx).First(&row, "run_id = ?", string(id))
	if res.Error != nil {
		if errors.Is(res.Error, gorm.ErrRecordNotFound) {
			return rjobs.Status{}, errtypes.NotFound(fmt.Sprintf("run %q not found", id))
		}
		return rjobs.Status{}, errors.Wrap(res.Error, "rjobs sql: reading status failed")
	}
	return fromModel(row)
}

func (s *store) Close(ctx context.Context) error {
	db, err := s.db.DB()
	if err != nil {
		return err
	}
	return db.Close()
}

func toModel(st rjobs.Status) (*model.Run, error) {
	var result datatypes.JSON
	if st.Result != nil {
		b, err := json.Marshal(st.Result)
		if err != nil {
			return nil, errors.Wrap(err, "rjobs sql: marshalling result failed")
		}
		result = b
	}
	return &model.Run{
		RunID:      string(st.RunID),
		Job:        st.Job,
		State:      string(st.State),
		Attempt:    st.Attempt,
		EnqueuedAt: st.EnqueuedAt,
		StartedAt:  st.StartedAt,
		FinishedAt: st.FinishedAt,
		LastError:  st.LastError,
		Result:     result,
	}, nil
}

func fromModel(row model.Run) (rjobs.Status, error) {
	st := rjobs.Status{
		RunID:      rjobs.RunID(row.RunID),
		Job:        row.Job,
		State:      rjobs.State(row.State),
		Attempt:    row.Attempt,
		EnqueuedAt: row.EnqueuedAt,
		StartedAt:  row.StartedAt,
		FinishedAt: row.FinishedAt,
		LastError:  row.LastError,
	}
	if len(row.Result) > 0 {
		var p rjobs.Params
		if err := json.Unmarshal(row.Result, &p); err != nil {
			return rjobs.Status{}, errors.Wrap(err, "rjobs sql: unmarshalling result failed")
		}
		st.Result = p
	}
	return st, nil
}
