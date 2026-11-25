// Copyright 2018-2024 CERN
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
	"bytes"
	"context"
	dbsql "database/sql"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	model "github.com/cs3org/reva/v3/pkg/share/manager/sql/model"
	"github.com/rs/zerolog"
	"gorm.io/gorm"
)

// commitFault replaces the acknowledgment of the next commit. The fault
// applies at the connection pool, after or instead of the real commit; it
// is not a network or crash experiment.
type commitFault struct {
	*dbsql.DB
	apply    bool         // run the real commit before failing
	between  func()       // runs after the commit outcome is decided
	failRead bool         // fail the reads that follow the fault
	armed    atomic.Bool  // the next commit is faulted
	faulted  atomic.Bool  // a commit was faulted
	commits  atomic.Int32 // commits attempted before the fault
	reads    atomic.Int32 // reads after the fault
	writes   atomic.Int32 // writes after the fault
}

var errLostAck = errors.New("commit acknowledgment lost")

func (p *commitFault) BeginTx(ctx context.Context, opts *dbsql.TxOptions) (gorm.ConnPool, error) {
	tx, err := p.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &faultTx{Tx: tx, p: p}, nil
}

// read counts a read after the fault and reports whether it must fail.
func (p *commitFault) read() bool {
	if !p.faulted.Load() {
		return false
	}
	p.reads.Add(1)
	return p.failRead
}

func (p *commitFault) QueryContext(ctx context.Context, q string, args ...any) (*dbsql.Rows, error) {
	if p.read() {
		return nil, errors.New("recovery read failed")
	}
	return p.DB.QueryContext(ctx, q, args...)
}

func (p *commitFault) QueryRowContext(ctx context.Context, q string, args ...any) *dbsql.Row {
	if p.read() {
		return p.DB.QueryRowContext(ctx, "SELECT * FROM no_such_table_for_the_fault")
	}
	return p.DB.QueryRowContext(ctx, q, args...)
}

func (p *commitFault) ExecContext(ctx context.Context, q string, args ...any) (dbsql.Result, error) {
	if p.faulted.Load() {
		p.writes.Add(1)
	}
	return p.DB.ExecContext(ctx, q, args...)
}

type faultTx struct {
	*dbsql.Tx
	p *commitFault
}

func (t *faultTx) QueryContext(ctx context.Context, q string, args ...any) (*dbsql.Rows, error) {
	if t.p.read() {
		return nil, errors.New("recovery read failed")
	}
	return t.Tx.QueryContext(ctx, q, args...)
}

func (t *faultTx) QueryRowContext(ctx context.Context, q string, args ...any) *dbsql.Row {
	if t.p.read() {
		return t.Tx.QueryRowContext(ctx, "SELECT * FROM no_such_table_for_the_fault")
	}
	return t.Tx.QueryRowContext(ctx, q, args...)
}

func (t *faultTx) ExecContext(ctx context.Context, q string, args ...any) (dbsql.Result, error) {
	if t.p.faulted.Load() {
		t.p.writes.Add(1)
	}
	return t.Tx.ExecContext(ctx, q, args...)
}

func (t *faultTx) Commit() error {
	if t.p.faulted.Load() {
		return t.Tx.Commit()
	}
	t.p.commits.Add(1)
	if !t.p.armed.CompareAndSwap(true, false) {
		return t.Tx.Commit()
	}
	if t.p.apply {
		if err := t.Tx.Commit(); err != nil {
			return err
		}
	} else {
		_ = t.Tx.Rollback()
	}
	if t.p.between != nil {
		t.p.between()
	}
	t.p.faulted.Store(true)
	return errLostAck
}

// withCommitFault routes the transactions of m through a faulting pool.
func withCommitFault(t *testing.T, m *mgr, p *commitFault) {
	t.Helper()
	sqlDB, err := m.db.DB()
	if err != nil {
		t.Fatal(err)
	}
	p.DB = sqlDB
	m.db.Statement.ConnPool = p
}

// other writes through a separate manager, like another instance.
func other(t *testing.T, cfg map[string]any) *gorm.DB {
	return openOCMManager(t, cfg).db
}

// rowOf returns the ID of the received share of remote for marie, or 0.
func rowOf(t *testing.T, db *gorm.DB, remote string) uint {
	t.Helper()
	var ids []uint
	if err := db.Model(&model.OcmReceivedShare{}).Where("remote_share_id = ? AND share_with = ?", remote, "marie").Order("id DESC").Pluck("id", &ids).Error; err != nil {
		t.Fatal(err)
	}
	if len(ids) == 0 {
		return 0
	}
	return ids[0]
}

func setRow(t *testing.T, db *gorm.DB, id uint, updates map[string]any) {
	t.Helper()
	if err := db.Model(&model.OcmReceivedShare{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		t.Fatal(err)
	}
}

func setProtocolURI(t *testing.T, db *gorm.DB, id uint) {
	t.Helper()
	if err := db.Model(&model.OcmReceivedShareProtocol{}).Where("ocm_received_share_id = ?", id).Update("uri", "https://other.example.org/dav/x").Error; err != nil {
		t.Fatal(err)
	}
}

func TestUnacknowledgedCommitsObserveTheExactProjection(t *testing.T) {
	marie := recipient("marie")
	type op struct {
		name string
		// setup prepares the share and returns what run needs.
		setup func(t *testing.T, m *mgr, remote string) (*ocm.ReceivedShare, uint64)
		run   func(ctx context.Context, m *mgr, rs *ocm.ReceivedShare, remote string, gen uint64) (bool, error)
		// done reports whether the requested state is durable.
		done func(t *testing.T, db *gorm.DB, remote string) bool
		// compete reaches the requested state with other values.
		compete func(t *testing.T, db *gorm.DB, remote string)
		// generation is logged with an unknown outcome.
		generation bool
	}
	stored := func(t *testing.T, m *mgr, remote string) (*ocm.ReceivedShare, uint64) {
		return mustStore(t, m, grantShare(marie, "a.example.org", remote, file)), 0
	}
	stateIs := func(state model.OcmShareState) func(t *testing.T, db *gorm.DB, remote string) bool {
		return func(t *testing.T, db *gorm.DB, remote string) bool {
			id := rowOf(t, db, remote)
			var row model.OcmReceivedShare
			return id != 0 && db.Where("id = ?", id).Take(&row).Error == nil && row.State == state
		}
	}
	ops := []op{
		{
			name:  "create",
			setup: func(t *testing.T, m *mgr, remote string) (*ocm.ReceivedShare, uint64) { return nil, 0 },
			run: func(ctx context.Context, m *mgr, _ *ocm.ReceivedShare, remote string, _ uint64) (bool, error) {
				got, err := m.StoreReceivedShare(ctx, grantShare(marie, "a.example.org", remote, file))
				return got != nil, err
			},
			done: func(t *testing.T, db *gorm.DB, remote string) bool { return rowOf(t, db, remote) != 0 },
		},
		{
			name:  "update state",
			setup: stored,
			run: func(ctx context.Context, m *mgr, rs *ocm.ReceivedShare, _ string, _ uint64) (bool, error) {
				u, err := m.UpdateReceivedShare(ctx, marie, &ocm.ReceivedShare{Id: rs.Id, State: ocm.ShareState_SHARE_STATE_ACCEPTED}, stateMask())
				return u != nil, err
			},
			done: stateIs(model.OcmShareStateAccepted),
			compete: func(t *testing.T, db *gorm.DB, remote string) {
				setRow(t, db, rowOf(t, db, remote), map[string]any{"state": model.OcmShareStateAccepted, "transfer_generation": 1, "mtime": 1})
			},
			generation: true,
		},
		{
			name:  "start transfer",
			setup: stored,
			run: func(ctx context.Context, m *mgr, rs *ocm.ReceivedShare, _ string, _ uint64) (bool, error) {
				s, err := m.StartReceivedShareTransfer(ctx, marie, &ocm.ReceivedShare{Id: rs.Id}, stateMask())
				return s != nil, err
			},
			done: stateIs(model.OcmShareStateTransferring),
			compete: func(t *testing.T, db *gorm.DB, remote string) {
				setRow(t, db, rowOf(t, db, remote), map[string]any{"state": model.OcmShareStateTransferring, "transfer_generation": 1, "mtime": 1})
			},
			generation: true,
		},
		{
			name: "complete transfer",
			setup: func(t *testing.T, m *mgr, remote string) (*ocm.ReceivedShare, uint64) {
				rs := mustStore(t, m, grantShare(marie, "a.example.org", remote, file))
				start, err := m.StartReceivedShareTransfer(context.Background(), marie, &ocm.ReceivedShare{Id: rs.Id}, stateMask())
				if err != nil {
					t.Fatal(err)
				}
				return rs, start.Generation
			},
			run: func(ctx context.Context, m *mgr, rs *ocm.ReceivedShare, _ string, gen uint64) (bool, error) {
				u, err := m.CompleteReceivedShareTransfer(ctx, marie, rs.Id, gen, ocm.ShareState_SHARE_STATE_ACCEPTED)
				return u != nil, err
			},
			done: stateIs(model.OcmShareStateAccepted),
			compete: func(t *testing.T, db *gorm.DB, remote string) {
				setRow(t, db, rowOf(t, db, remote), map[string]any{"state": model.OcmShareStateAccepted, "mtime": 1})
			},
			generation: true,
		},
	}
	cases := []struct {
		name     string
		apply    bool
		failRead bool
		// change runs after the commit outcome is decided.
		change   func(t *testing.T, o op, db *gorm.DB, remote string)
		observed share.CommitObservation
		durable  bool
	}{
		{name: "applied", apply: true, observed: share.CommitObservedRequested, durable: true},
		{name: "not applied", observed: share.CommitObservedPrevious},
		{name: "recovery read fails", apply: true, failRead: true, observed: share.CommitObservedUnread, durable: true},
		{name: "same state by another writer", change: func(t *testing.T, o op, db *gorm.DB, remote string) {
			o.compete(t, db, remote)
		}, observed: share.CommitObservedOther, durable: true},
		{name: "applied then row changed", apply: true, change: func(t *testing.T, o op, db *gorm.DB, remote string) {
			setRow(t, db, rowOf(t, db, remote), map[string]any{"mtime": 2})
		}, observed: share.CommitObservedOther, durable: true},
		{name: "applied then protocol changed", apply: true, change: func(t *testing.T, o op, db *gorm.DB, remote string) {
			setProtocolURI(t, db, rowOf(t, db, remote))
		}, observed: share.CommitObservedOther, durable: true},
	}
	for _, o := range ops {
		t.Run(o.name, func(t *testing.T) {
			forEachDialectConfig(t, func(t *testing.T, cfg map[string]any) {
				run := func(t *testing.T, m *mgr, rs *ocm.ReceivedShare, remote string, gen uint64) (bool, error, string) {
					var logs bytes.Buffer
					logger := zerolog.New(&logs)
					ctx := appctx.WithLogger(context.Background(), &logger)
					got, err := o.run(ctx, m, rs, remote, gen)
					return got, err, logs.String()
				}
				t.Run("acknowledged", func(t *testing.T) {
					m := openOCMManager(t, cfg)
					remote := "ack-" + strings.ReplaceAll(o.name, " ", "-")
					rs, gen := o.setup(t, m, remote)
					p := &commitFault{}
					withCommitFault(t, m, p)
					got, err, _ := run(t, m, rs, remote, gen)
					if err != nil || !got || !o.done(t, m.db, remote) || p.reads.Load() != 0 {
						t.Fatalf("receipt=%t err=%v reads=%d", got, err, p.reads.Load())
					}
				})
				for _, tc := range cases {
					if tc.name == "same state by another writer" && o.compete == nil {
						continue
					}
					t.Run(tc.name, func(t *testing.T) {
						m := openOCMManager(t, cfg)
						remote := strings.ReplaceAll(o.name+"-"+tc.name, " ", "-")
						rs, gen := o.setup(t, m, remote)
						p := &commitFault{apply: tc.apply, failRead: tc.failRead}
						if tc.change != nil {
							p.between = func() { tc.change(t, o, other(t, cfg), remote) }
						}
						withCommitFault(t, m, p)
						p.armed.Store(true)
						got, err, logs := run(t, m, rs, remote, gen)
						var unknown *share.CommitUnknownError
						if got || !errors.As(err, &unknown) || !errors.Is(err, share.ErrCommitUnknown) || unknown.Observed != tc.observed {
							t.Fatalf("receipt=%t err=%v", got, err)
						}
						if p.commits.Load() != 1 || p.writes.Load() != 0 {
							t.Fatalf("after an unacknowledged commit: commits %d, writes %d", p.commits.Load(), p.writes.Load())
						}
						if p.reads.Load() == 0 {
							t.Fatal("no reconciliation read")
						}
						key, _ := share.GrantKey("a.example.org", remote)
						for _, want := range []string{`"observed":"` + string(tc.observed) + `"`, `"grant_key":"` + key + `"`} {
							if !strings.Contains(logs, want) {
								t.Fatalf("log lacks %s: %s", want, logs)
							}
						}
						if o.generation && !strings.Contains(logs, `"transfer_generation"`) {
							t.Fatalf("log lacks the generation: %s", logs)
						}
						if o.name != "create" && !strings.Contains(logs, `"share_id":`+rs.Id.OpaqueId) {
							t.Fatalf("log lacks the share ID: %s", logs)
						}
						if strings.Contains(logs, "secret-") || strings.Contains(logs, "dav/") || strings.Contains(logs, "marie") {
							t.Fatalf("log holds share data: %s", logs)
						}
						p.failRead = false
						if o.done(t, other(t, cfg), remote) != tc.durable {
							t.Fatalf("durable change %t, want %t", !tc.durable, tc.durable)
						}
					})
				}
			})
		})
	}
}

// An older row of the same grant and user is not the row a failed creation
// allocated, so it is not its requested state.
func TestUnacknowledgedCreateIgnoresAnOlderRowOfTheGrant(t *testing.T) {
	forEachDialectConfig(t, func(t *testing.T, cfg map[string]any) {
		m := openOCMManager(t, cfg)
		older := mustStore(t, m, grantShare(recipient("marie"), "a.example.org", "dup", file))
		p := &commitFault{}
		withCommitFault(t, m, p)
		p.armed.Store(true)
		in := grantShare(recipient("marie"), "a.example.org", "dup", file)
		got, err := m.StoreReceivedShare(context.Background(), in)
		var unknown *share.CommitUnknownError
		if got != nil || !errors.As(err, &unknown) || unknown.Observed != share.CommitObservedPrevious {
			t.Fatalf("%v %v", got, err)
		}
		if in.Id != nil {
			t.Fatalf("unconfirmed creation published ID %v", in.Id)
		}
		if id := rowOf(t, other(t, cfg), "dup"); fmt.Sprint(id) != older.Id.OpaqueId {
			t.Fatalf("rows changed: newest %d", id)
		}
	})
}

// A completion of a finished attempt changes nothing, so its unknown commit
// is never observed as the requested completion.
func TestUnacknowledgedNoOpCompletionIsNotACompletion(t *testing.T) {
	forEachDialectConfig(t, func(t *testing.T, cfg map[string]any) {
		m := openOCMManager(t, cfg)
		marie := recipient("marie")
		rs := mustStore(t, m, grantShare(marie, "a.example.org", "noop", file))
		start, err := m.StartReceivedShareTransfer(context.Background(), marie, &ocm.ReceivedShare{Id: rs.Id}, stateMask())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := m.CompleteReceivedShareTransfer(context.Background(), marie, rs.Id, start.Generation, ocm.ShareState_SHARE_STATE_ACCEPTED); err != nil {
			t.Fatal(err)
		}
		p := &commitFault{apply: true}
		withCommitFault(t, m, p)
		p.armed.Store(true)
		u, err := m.CompleteReceivedShareTransfer(context.Background(), marie, rs.Id, start.Generation, ocm.ShareState_SHARE_STATE_ACCEPTED)
		var unknown *share.CommitUnknownError
		if u != nil || !errors.As(err, &unknown) || unknown.Observed != share.CommitObservedPrevious {
			t.Fatalf("%v %v", u, err)
		}
	})
}

func TestUnacknowledgedEqualProjectionsObservePrevious(t *testing.T) {
	forEachDialectConfig(t, func(t *testing.T, cfg map[string]any) {
		for _, apply := range []bool{false, true} {
			t.Run(fmt.Sprintf("applied=%t", apply), func(t *testing.T) {
				m := openOCMManager(t, cfg)
				rs := mustStore(t, m, grantShare(recipient("marie"), "a.example.org", fmt.Sprint("same-", apply), file))
				row := receivedRow(t, m, rs.Id.OpaqueId)
				fault := &commitFault{apply: apply}
				withCommitFault(t, m, fault)
				fault.armed.Store(true)
				var check commitCheck
				err := m.transact(context.Background(), "test_equal_projections", func(tx *gorm.DB) error {
					previous, err := readReceivedProjection(tx, row.ID)
					if err != nil {
						return err
					}
					expected, err := readReceivedProjection(tx, row.ID)
					if err != nil {
						return err
					}
					check = receivedCheck(&row, expected, previous)
					return nil
				}, &check)
				var unknown *share.CommitUnknownError
				if !errors.As(err, &unknown) || unknown.Observed != share.CommitObservedPrevious ||
					fault.commits.Load() != 1 || fault.writes.Load() != 0 {
					t.Fatalf("same durable projection: %v, commits=%d, recovery writes=%d", err, fault.commits.Load(), fault.writes.Load())
				}
			})
		}
	})
}
