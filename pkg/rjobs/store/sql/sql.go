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
	"time"

	"github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/rjobs"
	"github.com/cs3org/reva/v3/pkg/rjobs/store/sql/model"
	"github.com/pkg/errors"
	"gorm.io/datatypes"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	// later transition. The reservation column is owned by Reserve/Release, the
	// cancel intent by RequestCancel and the progress by PutProgress, so they are
	// omitted here: a lifecycle write must never wipe a live reservation, clobber
	// a concurrent cancel or drop the progress.
	res := s.db.WithContext(ctx).Omit("ActiveDedupKey", "CancelRequested", "Progress", "ProgressAt").Save(row)
	if res.Error != nil {
		return errors.Wrap(res.Error, "rjobs sql: storing status failed")
	}
	return nil
}

func (s *store) RequestCancel(ctx context.Context, id rjobs.RunID) (rjobs.Status, error) {
	// Flip the cancel intent with a targeted update that leaves the columns the
	// worker owns (lifecycle state, timestamps, result) untouched. The state is
	// advanced to cancelling only from a non-terminal state, so a cancel can
	// neither resurrect a finished run nor undo a terminal state.
	res := s.db.WithContext(ctx).Model(&model.Run{}).
		Where("run_id = ? AND state IN ?", string(id), []string{
			string(rjobs.StateQueued), string(rjobs.StateRunning), string(rjobs.StateFailed),
		}).
		Updates(map[string]any{
			"cancel_requested": true,
			"state":            string(rjobs.StateCancelling),
		})
	if res.Error != nil {
		return rjobs.Status{}, errors.Wrap(res.Error, "rjobs sql: requesting cancel failed")
	}
	// Return the current status whether or not a row was updated: a terminal run
	// is returned unchanged (idempotent cancel) and an unknown run yields
	// NotFound from Get.
	return s.Get(ctx, id)
}

// PutProgress records the latest progress of a run that is not terminal yet, so
// a late write can never replace the final snapshot.
func (s *store) PutProgress(ctx context.Context, id rjobs.RunID, p rjobs.Progress, at time.Time) error {
	b, err := json.Marshal(p)
	if err != nil {
		return errors.Wrap(err, "rjobs sql: marshalling progress failed")
	}
	res := s.db.WithContext(ctx).Model(&model.Run{}).
		Where("run_id = ? AND state NOT IN ?", string(id), terminalStates).
		Updates(map[string]any{
			"progress":    datatypes.JSON(b),
			"progress_at": at,
		})
	if res.Error != nil {
		return errors.Wrap(res.Error, "rjobs sql: storing progress failed")
	}
	return nil
}

// terminalStates are the states a run never leaves.
var terminalStates = []string{string(rjobs.StateSucceeded), string(rjobs.StateCancelled), string(rjobs.StateAborted)}

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

func (s *store) List(ctx context.Context, f rjobs.ListFilter) ([]rjobs.Status, error) {
	q := s.db.WithContext(ctx).Model(&model.Run{})
	switch {
	case f.Internal:
		q = q.Where("owner = ?", "")
	case f.Owner != "":
		q = q.Where("owner = ?", f.Owner)
	}
	if len(f.States) > 0 {
		states := make([]string, len(f.States))
		for i, st := range f.States {
			states[i] = string(st)
		}
		q = q.Where("state IN ?", states)
	}
	if f.Job != "" {
		q = q.Where("job = ?", f.Job)
	}
	q = q.Order("enqueued_at DESC")
	if f.Limit > 0 {
		q = q.Limit(f.Limit)
	}
	if f.Offset > 0 {
		q = q.Offset(f.Offset)
	}

	var rows []model.Run
	if err := q.Find(&rows).Error; err != nil {
		return nil, errors.Wrap(err, "rjobs sql: listing runs failed")
	}
	out := make([]rjobs.Status, 0, len(rows))
	for _, row := range rows {
		st, err := fromModel(row)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, nil
}

func (s *store) Reserve(ctx context.Context, st rjobs.Status, key string) (rjobs.Status, bool, error) {
	// Retry to cover the narrow window where the current holder releases the key
	// between our insert seeing it taken and the lookup of that holder.
	for range 5 {
		row, err := toModel(st)
		if err != nil {
			return rjobs.Status{}, false, err
		}
		row.ActiveDedupKey = &key

		// Insert unless (owner, key) is already taken: a unique-index conflict is
		// the expected "key already held" case, so the database skips it instead
		// of erroring. RowsAffected == 1 means our row went in — we won the key.
		res := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(row)
		if res.Error != nil {
			return rjobs.Status{}, false, errors.Wrap(res.Error, "rjobs sql: reserving run failed")
		}
		if res.RowsAffected == 1 {
			return rjobs.Status{}, true, nil
		}

		holder, found, err := s.activeHolder(ctx, st.Owner, key)
		if err != nil {
			return rjobs.Status{}, false, err
		}
		if found {
			return holder, false, nil
		}
		// the holder released the key just now; loop and try to take it.
	}
	return rjobs.Status{}, false, errors.New("rjobs sql: could not reserve run, key is contended")
}

func (s *store) Release(ctx context.Context, id rjobs.RunID) error {
	res := s.db.WithContext(ctx).Model(&model.Run{}).
		Where("run_id = ?", string(id)).
		Update("active_dedup_key", nil)
	if res.Error != nil {
		return errors.Wrap(res.Error, "rjobs sql: releasing reservation failed")
	}
	return nil
}

// activeHolder loads the run currently holding key for owner, if any.
func (s *store) activeHolder(ctx context.Context, owner, key string) (rjobs.Status, bool, error) {
	q := s.db.WithContext(ctx).Where("owner = ? AND active_dedup_key = ?", owner, key)
	var row model.Run
	if err := q.First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return rjobs.Status{}, false, nil
		}
		return rjobs.Status{}, false, errors.Wrap(err, "rjobs sql: loading reservation holder failed")
	}
	st, err := fromModel(row)
	if err != nil {
		return rjobs.Status{}, false, err
	}
	return st, true, nil
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
	m := &model.Run{
		RunID:           string(st.RunID),
		Job:             st.Job,
		State:           string(st.State),
		Attempt:         st.Attempt,
		EnqueuedAt:      st.EnqueuedAt,
		StartedAt:       st.StartedAt,
		FinishedAt:      st.FinishedAt,
		LastError:       st.LastError,
		Result:          result,
		CancelRequested: st.CancelRequested,
	}
	m.Owner = st.Owner
	return m, nil
}

func fromModel(row model.Run) (rjobs.Status, error) {
	st := rjobs.Status{
		RunID:           rjobs.RunID(row.RunID),
		Job:             row.Job,
		State:           rjobs.State(row.State),
		Attempt:         row.Attempt,
		EnqueuedAt:      row.EnqueuedAt,
		StartedAt:       row.StartedAt,
		FinishedAt:      row.FinishedAt,
		LastError:       row.LastError,
		CancelRequested: row.CancelRequested,
		ProgressAt:      row.ProgressAt,
	}
	st.Owner = row.Owner
	if len(row.Progress) > 0 {
		var p rjobs.Progress
		if err := json.Unmarshal(row.Progress, &p); err != nil {
			return rjobs.Status{}, errors.Wrap(err, "rjobs sql: unmarshalling progress failed")
		}
		st.Progress = &p
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
