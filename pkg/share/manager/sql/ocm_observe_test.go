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

package sql

import (
	"context"
	dbsql "database/sql"
	"errors"
	"fmt"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cs3org/reva/v3/pkg/ocm/share"
	model "github.com/cs3org/reva/v3/pkg/share/manager/sql/model"
	"github.com/mattn/go-sqlite3"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

const newURI = "https://a.example.org/dav/new"

// observer opens a separate pool for the observation. On MySQL every session
// of the pool starts with the given default isolation.
func observer(t *testing.T, cfg map[string]any, isolation string) *gorm.DB {
	t.Helper()
	if cfg["db_engine"] != "mysql" {
		return other(t, cfg)
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&transaction_isolation=%s",
		cfg["db_username"], cfg["db_password"], cfg["db_host"], cfg["db_port"], cfg["db_name"], url.QueryEscape("'"+isolation+"'"))
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	var got string
	if err := db.Raw("SELECT @@SESSION.transaction_isolation").Scan(&got).Error; err != nil || got != isolation {
		t.Fatalf("observer session isolation %q: %v", got, err)
	}
	return db
}

// changeAtomically renames the share and replaces its protocol URI.
func changeAtomically(db *gorm.DB, id uint) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.OcmReceivedShare{}).Where("id = ?", id).Update("name", "renamed").Error; err != nil {
			return err
		}
		return tx.Model(&model.OcmReceivedShareProtocol{}).Where("ocm_received_share_id = ?", id).Update("uri", newURI).Error
	})
}

// mixture is the share as it never existed: its old row with the new protocol.
func mixture(p *receivedProjection) *receivedProjection {
	m := &receivedProjection{row: p.row, protocols: append([]model.OcmReceivedShareProtocol(nil), p.protocols...)}
	for i := range m.protocols {
		m.protocols[i].Uri = newURI
	}
	return m
}

func mustRead[P any](t *testing.T, read func(*gorm.DB) (P, error), db *gorm.DB) P {
	t.Helper()
	p, err := read(db)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// Each case changes the share between the observer's SELECTs and expects a
// combination of old and new rows that never existed. A coherent observation
// sees one state and never reports that mixture as requested.
func TestObservationNeverCombinesTwoStates(t *testing.T) {
	marie := recipient("marie")
	for _, isolation := range []string{"READ-COMMITTED", "REPEATABLE-READ"} {
		t.Run(isolation, func(t *testing.T) {
			forEachDialectConfig(t, func(t *testing.T, cfg map[string]any) {
				if cfg["db_engine"] != "mysql" && isolation != "READ-COMMITTED" {
					t.Skip("SQLite has a single isolation")
				}
				m := openOCMManager(t, cfg)
				readShare := func(id uint) func(*gorm.DB) (*receivedProjection, error) {
					return func(db *gorm.DB) (*receivedProjection, error) { return readReceivedProjection(db, id) }
				}

				t.Run("received", func(t *testing.T) {
					mustStore(t, m, grantShare(marie, "a.example.org", "r1", file))
					id := rowOf(t, m.db, "r1")
					before := mustRead(t, readShare(id), m.db)
					expected := mixture(before)
					obs := observer(t, cfg, isolation)
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					sqlite := cfg["db_engine"] == "sqlite"
					if sqlite {
						pool, err := m.db.DB()
						if err != nil {
							t.Fatal(err)
						}
						pool.SetMaxOpenConns(1)
						if _, err := pool.ExecContext(ctx, "PRAGMA busy_timeout = 0"); err != nil {
							t.Fatal(err)
						}
					}
					var attempted bool
					var writeErr error
					if err := obs.Callback().Query().After("gorm:query").Register("test:interleave", func(*gorm.DB) {
						if !attempted {
							attempted = true
							writeErr = changeAtomically(m.db.WithContext(ctx), id)
						}
					}); err != nil {
						t.Fatal(err)
					}
					got, err := observeReceived(obs.WithContext(ctx), id, expected, before)
					if !attempted {
						t.Fatal("observer made no query")
					}
					if sqlite {
						var sqliteErr sqlite3.Error
						if !errors.As(writeErr, &sqliteErr) || sqliteErr.Code != sqlite3.ErrBusy {
							t.Fatalf("writer during immediate observation: %v, want SQLITE_BUSY", writeErr)
						}
						writeErr = changeAtomically(m.db.WithContext(ctx), id)
					}
					if writeErr != nil {
						t.Fatal(writeErr)
					}
					after := mustRead(t, readShare(id), m.db)
					if after.row.Name != "renamed" || after.protocols[0].Uri != newURI || before.equal(expected) || after.equal(expected) {
						t.Fatal("the writer did not commit a state different from the mixture")
					}
					if err != nil || got != share.CommitObservedPrevious {
						t.Fatalf("observed %q %v, want previous snapshot", got, err)
					}
				})

			})
		})
	}
}

// observeFault fails the begin or the commit of the observation transaction.
type observeFault struct {
	*dbsql.DB
	failBegin, failCommit bool
	reads                 atomic.Int32
}

var errObserveFault = errors.New("observation transaction failed")

func (p *observeFault) BeginTx(ctx context.Context, opts *dbsql.TxOptions) (gorm.ConnPool, error) {
	if p.failBegin {
		return nil, errObserveFault
	}
	tx, err := p.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &observeFaultTx{Tx: tx, p: p}, nil
}

func (p *observeFault) QueryContext(ctx context.Context, q string, args ...any) (*dbsql.Rows, error) {
	p.reads.Add(1)
	return p.DB.QueryContext(ctx, q, args...)
}

type observeFaultTx struct {
	*dbsql.Tx
	p *observeFault
}

func (t *observeFaultTx) Commit() error {
	if t.p.failCommit {
		_ = t.Tx.Rollback()
		return errObserveFault
	}
	return t.Tx.Commit()
}

// A failed begin or commit of the observation leaves it unread, with no
// read outside the transaction and no change to the share.
func TestObservationTransactionFailureIsUnread(t *testing.T) {
	marie := recipient("marie")
	forEachDialectConfig(t, func(t *testing.T, cfg map[string]any) {
		m := openOCMManager(t, cfg)
		mustStore(t, m, grantShare(marie, "a.example.org", "r1", file))
		id := rowOf(t, m.db, "r1")
		before, err := readReceivedProjection(m.db, id)
		if err != nil {
			t.Fatal(err)
		}
		for _, fault := range []*observeFault{{failBegin: true}, {failCommit: true}} {
			obs := openOCMManager(t, cfg)
			sqlDB, err := obs.db.DB()
			if err != nil {
				t.Fatal(err)
			}
			fault.DB = sqlDB
			obs.db.Statement.ConnPool = fault
			checks := []*commitCheck{
				{shareID: id, observe: func(db *gorm.DB) (share.CommitObservation, error) {
					return observeReceived(db, id, before, before)
				}},
			}
			for _, check := range checks {
				err := obs.reconcileCommit(context.Background(), "test", check)
				var unknown *share.CommitUnknownError
				if !errors.As(err, &unknown) || unknown.Observed != share.CommitObservedUnread {
					t.Fatalf("begin=%v commit=%v: %v", fault.failBegin, fault.failCommit, err)
				}
			}
			if fault.reads.Load() != 0 {
				t.Fatalf("%d reads outside the observation", fault.reads.Load())
			}
		}
		after, err := readReceivedProjection(m.db, id)
		if err != nil || !after.equal(before) {
			t.Fatalf("a failed observation changed the grant: %v", err)
		}
	})
}
