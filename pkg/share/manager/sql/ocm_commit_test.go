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
	"context"
	dbsql "database/sql"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	model "github.com/cs3org/reva/v3/pkg/share/manager/sql/model"
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
	commits  atomic.Int32 // commits attempted
	reads    atomic.Int32 // reads after the fault
}

var errLostAck = errors.New("commit acknowledgment lost")

func (p *commitFault) BeginTx(ctx context.Context, opts *dbsql.TxOptions) (gorm.ConnPool, error) {
	tx, err := p.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &faultTx{Tx: tx, p: p}, nil
}

func (p *commitFault) QueryContext(ctx context.Context, q string, args ...any) (*dbsql.Rows, error) {
	if p.faulted.Load() {
		p.reads.Add(1)
		if p.failRead {
			return nil, errors.New("recovery read failed")
		}
	}
	return p.DB.QueryContext(ctx, q, args...)
}

func (p *commitFault) QueryRowContext(ctx context.Context, q string, args ...any) *dbsql.Row {
	if p.faulted.Load() {
		p.reads.Add(1)
		if p.failRead {
			return p.DB.QueryRowContext(ctx, "SELECT * FROM no_such_table_for_the_fault")
		}
	}
	return p.DB.QueryRowContext(ctx, q, args...)
}

type faultTx struct {
	*dbsql.Tx
	p *commitFault
}

func (t *faultTx) Commit() error {
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

func TestUnacknowledgedCommitsAreReconciled(t *testing.T) {
	type op struct {
		name string
		// run performs the operation and reports whether it returned a receipt.
		run func(t *testing.T, m *mgr, rs *ocm.ReceivedShare) (bool, error)
		// done reports whether the change is durable.
		done func(t *testing.T, m *mgr, rs *ocm.ReceivedShare) bool
		// compete makes the same change through another connection.
		compete func(t *testing.T, m *mgr, rs *ocm.ReceivedShare)
	}
	accept := &ocm.ReceivedShare{State: ocm.ShareState_SHARE_STATE_ACCEPTED}
	ops := []op{
		{
			name: "update state",
			run: func(t *testing.T, m *mgr, rs *ocm.ReceivedShare) (bool, error) {
				u, err := m.UpdateReceivedShare(context.Background(), recipient("marie"), &ocm.ReceivedShare{Id: rs.Id, State: accept.State}, stateMask())
				return u != nil, err
			},
			done: func(t *testing.T, m *mgr, rs *ocm.ReceivedShare) bool {
				return receivedRow(t, m, rs.Id.OpaqueId).State == model.OcmShareStateAccepted
			},
			compete: func(t *testing.T, m *mgr, rs *ocm.ReceivedShare) {
				if err := m.db.Session(&gorm.Session{NewDB: true}).Model(&model.OcmReceivedShare{}).Where("id = ?", rs.Id.OpaqueId).
					Updates(map[string]any{"state": model.OcmShareStateAccepted, "transfer_generation": 1}).Error; err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "start transfer",
			run: func(t *testing.T, m *mgr, rs *ocm.ReceivedShare) (bool, error) {
				s, err := m.StartReceivedShareTransfer(context.Background(), recipient("marie"), &ocm.ReceivedShare{Id: rs.Id}, stateMask())
				return s != nil, err
			},
			done: func(t *testing.T, m *mgr, rs *ocm.ReceivedShare) bool {
				return receivedRow(t, m, rs.Id.OpaqueId).State == model.OcmShareStateTransferring
			},
			compete: func(t *testing.T, m *mgr, rs *ocm.ReceivedShare) {
				if err := m.db.Session(&gorm.Session{NewDB: true}).Model(&model.OcmReceivedShare{}).Where("id = ?", rs.Id.OpaqueId).
					Updates(map[string]any{"state": model.OcmShareStateTransferring, "transfer_generation": 1}).Error; err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "withdraw",
			run: func(t *testing.T, m *mgr, rs *ocm.ReceivedShare) (bool, error) {
				r, err := m.WithdrawReceivedGrant(context.Background(), withdrawal("a.example.org", rs.RemoteShareId))
				return r != nil, err
			},
			done: func(t *testing.T, m *mgr, rs *ocm.ReceivedShare) bool {
				ident, _ := identityRow(t, m, "a.example.org", rs.RemoteShareId)
				return ident.WithdrawnAt.Valid
			},
			compete: func(t *testing.T, m *mgr, rs *ocm.ReceivedShare) {
				if _, err := m.WithdrawReceivedGrant(context.Background(), withdrawal("a.example.org", rs.RemoteShareId)); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	cases := []struct {
		name     string
		apply    bool
		failRead bool
		compete  bool
		observed share.CommitObservation
		durable  bool
	}{
		{name: "applied", apply: true, observed: share.CommitObservedRequested, durable: true},
		{name: "not applied", observed: share.CommitObservedPrevious},
		{name: "recovery read fails", apply: true, failRead: true, observed: share.CommitObservedUnread, durable: true},
		{name: "competing writer", compete: true, observed: share.CommitObservedRequested, durable: true},
	}
	for _, o := range ops {
		t.Run(o.name, func(t *testing.T) {
			forEachDialectConfig(t, func(t *testing.T, cfg map[string]any) {
				t.Run("acknowledged", func(t *testing.T) {
					m := openOCMManager(t, cfg)
					rs := mustStore(t, m, grantShare(recipient("marie"), "a.example.org", "ack-"+o.name, file))
					p := &commitFault{}
					withCommitFault(t, m, p)
					got, err := o.run(t, m, rs)
					if err != nil || !got || !o.done(t, m, rs) || p.reads.Load() != 0 {
						t.Fatalf("receipt=%t err=%v reads=%d", got, err, p.reads.Load())
					}
				})
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						m := openOCMManager(t, cfg)
						rs := mustStore(t, m, grantShare(recipient("marie"), "a.example.org", strings.ReplaceAll(o.name+"-"+tc.name, " ", "-"), file))
						p := &commitFault{apply: tc.apply, failRead: tc.failRead}
						if tc.compete {
							p.between = func() { o.compete(t, openOCMManager(t, cfg), rs) }
						}
						withCommitFault(t, m, p)
						p.armed.Store(true)
						got, err := o.run(t, m, rs)
						var unknown *share.CommitUnknownError
						if got || !errors.As(err, &unknown) || !errors.Is(err, share.ErrCommitUnknown) || unknown.Observed != tc.observed {
							t.Fatalf("receipt=%t err=%v", got, err)
						}
						if p.commits.Load() != 1 {
							t.Fatalf("an unacknowledged commit was run %d times", p.commits.Load())
						}
						if p.reads.Load() == 0 {
							t.Fatal("no reconciliation read")
						}
						p.failRead = false
						if o.done(t, m, rs) != tc.durable {
							t.Fatalf("durable change %t, want %t", !tc.durable, tc.durable)
						}
					})
				}
			})
		})
	}
}
