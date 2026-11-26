// Copyright 2026 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.

package sql

import (
	"context"
	dbsql "database/sql"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/share/manager/sql/model"
	"github.com/go-sql-driver/mysql"
	"github.com/mattn/go-sqlite3"
	"gorm.io/gorm"
)

// transactionGate pauses at the actual commit or after an actual rollback.
// It is confined to the action's pool; the other manager uses a separate pool.
type transactionGate struct {
	*dbsql.DB
	mode           string
	reached        chan struct{}
	release        chan struct{}
	rollbackFailed chan error
	fired          atomic.Bool
}
type gatedTx struct {
	*dbsql.Tx
	gate *transactionGate
}

func (g *transactionGate) BeginTx(ctx context.Context, opts *dbsql.TxOptions) (gorm.ConnPool, error) {
	tx, err := g.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &gatedTx{Tx: tx, gate: g}, nil
}
func (t *gatedTx) Commit() error {
	if t.gate.mode == "commit" && t.gate.fired.CompareAndSwap(false, true) {
		close(t.gate.reached)
		<-t.gate.release
	}
	return t.Tx.Commit()
}
func (t *gatedTx) Rollback() error {
	err := t.Tx.Rollback()
	if t.gate.mode == "retry" && t.gate.fired.CompareAndSwap(false, true) {
		if err != nil {
			t.gate.rollbackFailed <- err
			return err
		}
		close(t.gate.reached)
		<-t.gate.release
	}
	return err
}

func TestDirectedTransactionConcurrentSelectionAndRetry(t *testing.T) {
	forEachDialectConfig(t, func(t *testing.T, cfg map[string]any) {
		for _, direction := range []string{"outgoing", "incoming"} {
			for _, mode := range []string{"commit", "retry"} {
				t.Run(direction+"/"+mode, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					m := openOCMManager(t, cfg)
					writer := openOCMManager(t, cfg)
					alice := &userpb.UserId{OpaqueId: "concurrency-alice-" + direction + "-" + mode, Idp: "directory", Type: userpb.UserType_USER_TYPE_PRIMARY}
					bob := &userpb.UserId{OpaqueId: "concurrency-bob", Idp: "remote.example.org", Type: userpb.UserType_USER_TYPE_FEDERATED}
					store := func(manager *mgr, token string, id *ocm.ShareId) (string, error) {
						if direction == "incoming" {
							s := grantShare(&userpb.User{Id: alice}, bob.Idp, token, file)
							s.Creator = bob
							got, err := manager.StoreReceivedShare(ctx, s)
							if err != nil {
								return "", err
							}
							return got.Id.OpaqueId, nil
						}
						g := &provider.Grantee{Type: provider.GranteeType_GRANTEE_TYPE_USER, Id: &provider.Grantee_UserId{UserId: bob}}
						s := getOcmShare(getOcmAccessMethods("viewer"), g, alice, &provider.ResourceId{StorageId: "storage", OpaqueId: token}, token)
						s.RecipientType = ocm.RecipientType_RECIPIENT_TYPE_USER
						if id == nil {
							var err error
							id, err = manager.GenerateID(ctx)
							if err != nil {
								return "", err
							}
						}
						s.Id = id
						_, err := manager.StoreShare(ctx, s, file)
						return id.OpaqueId, err
					}
					original, err := store(m, direction+mode+"original", nil)
					if err != nil {
						t.Fatal(err)
					}
					sqlite := cfg["db_engine"] == "sqlite"
					if mode == "commit" && !sqlite {
						// InnoDB gap locks protect the captured selection at this isolation.
						m.db = observer(t, cfg, "REPEATABLE-READ")
						t.Log("MySQL action pool session isolation: REPEATABLE-READ")
					}
					db, err := m.db.DB()
					if err != nil {
						t.Fatal(err)
					}
					gate := &transactionGate{
						DB: db, mode: mode, reached: make(chan struct{}), release: make(chan struct{}), rollbackFailed: make(chan error, 1),
					}
					release := sync.OnceFunc(func() { close(gate.release) })
					m.db.Statement.ConnPool = gate
					var laterID *ocm.ShareId
					if mode == "commit" && direction == "outgoing" {
						laterID, err = writer.GenerateID(ctx)
						if err != nil {
							t.Fatal(err)
						}
					}
					var injected atomic.Bool
					firstWriteFailed := make(chan error, 1)
					var outgoingBefore model.OcmShare
					var incomingBefore model.OcmReceivedShare
					if mode == "retry" {
						operation, table := "update", "ocm_received_shares"
						if direction == "outgoing" {
							operation, table = "delete", "ocm_shares"
							if err := writer.db.WithContext(ctx).Where("id = ?", original).Take(&outgoingBefore).Error; err != nil {
								t.Fatal(err)
							}
						} else {
							incomingBefore = receivedRow(t, writer, original)
						}
						callback := func(tx *gorm.DB) {
							if tx.Statement.Table != table || injected.Load() {
								return
							}
							if tx.Error != nil || tx.RowsAffected != 1 {
								firstWriteFailed <- fmt.Errorf("first %s on %s affected %d rows (want 1), error %v", operation, table, tx.RowsAffected, tx.Error)
								tx.Error = errors.New("unexpected first directed write result")
								return
							}
							injected.Store(true)
							tx.AddError(errors.New("database is locked: injected retryable failure after mutation"))
						}
						if direction == "outgoing" {
							err = m.db.Callback().Delete().After("gorm:delete").Register("test:retry-directed-delete", callback)
						} else {
							err = m.db.Callback().Update().After("gorm:update").Register("test:retry-directed-update", callback)
						}
						if err != nil {
							t.Fatal(err)
						}
					}
					type result struct {
						ids []string
						err error
					}
					done := make(chan result, 1)
					go func() {
						defer close(done)
						var out result
						if direction == "outgoing" {
							receipts, err := m.RemoveOCMSharesForRecipient(ctx, alice, bob)
							out.err = err
							for _, receipt := range receipts {
								out.ids = append(out.ids, receipt.Share.Id.OpaqueId)
							}
						} else {
							receipts, err := m.RejectReceivedOCMSharesFromSender(ctx, alice, bob)
							out.err = err
							for _, receipt := range receipts {
								out.ids = append(out.ids, receipt.After.Id.OpaqueId)
							}
						}
						done <- out
					}()
					defer func() {
						release()
						cancel()
						for range done {
						}
					}()
					var earlyAction *result
					var rollbackErr error
					select {
					case <-gate.reached:
					case action := <-done:
						earlyAction = &action
						select {
						case rollbackErr = <-gate.rollbackFailed:
						default:
						}
					case err := <-firstWriteFailed:
						t.Fatal(err)
					case rollbackErr = <-gate.rollbackFailed:
					case <-ctx.Done():
						t.Fatal("transaction did not reach barrier")
					}
					select {
					case err := <-firstWriteFailed:
						t.Fatal(err)
					default:
					}
					if rollbackErr != nil {
						t.Fatalf("rollback before retry barrier: %v", rollbackErr)
					}
					if earlyAction != nil {
						t.Fatalf("action returned before %s barrier: error %v, receipt IDs %v", mode, earlyAction.err, earlyAction.ids)
					}
					token := direction + mode + "later"
					if mode == "commit" {
						// The action cannot commit until a real competing write
						// has been refused by the database.
						var lockErr error
						if err := writer.db.WithContext(ctx).Connection(func(db *gorm.DB) error {
							setTimeout := "SET SESSION innodb_lock_wait_timeout = 1"
							readTimeout, want := "SELECT @@SESSION.innodb_lock_wait_timeout", 1
							if sqlite {
								setTimeout = "PRAGMA busy_timeout = 0"
								readTimeout, want = "PRAGMA busy_timeout", 0
							}
							if err := db.Exec(setTimeout).Error; err != nil {
								return err
							}
							var got int
							if err := db.Raw(readTimeout).Row().Scan(&got); err != nil {
								return err
							}
							if got != want {
								t.Fatalf("writer session lock wait timeout %d, want %d", got, want)
							}
							_, lockErr = store(&mgr{c: writer.c, db: db}, token, laterID)
							return nil
						}); err != nil {
							t.Fatalf("configure competing writer session: %v", err)
						}
						var sqliteErr sqlite3.Error
						var mysqlErr *mysql.MySQLError
						if sqlite {
							if !errors.As(lockErr, &sqliteErr) || sqliteErr.Code != sqlite3.ErrBusy {
								t.Fatalf("competing store during SQLite transaction: %v, want SQLITE_BUSY", lockErr)
							}
						} else if !errors.As(lockErr, &mysqlErr) || mysqlErr.Number != 1205 {
							t.Fatalf("competing store during InnoDB transaction: %v, want lock wait timeout", lockErr)
						}
						t.Logf("competing %s store before commit: %v", direction, lockErr)
						release()
					}
					var later string
					var writerErr error
					if mode == "retry" {
						if !injected.Load() {
							t.Fatal("rollback barrier reached without a successful first directed write")
						}
						if direction == "outgoing" {
							var restored model.OcmShare
							if err := writer.db.WithContext(ctx).Where("id = ?", original).Take(&restored).Error; err != nil {
								t.Fatalf("read original outgoing grant after rollback: %v", err)
							}
							if !reflect.DeepEqual(restored, outgoingBefore) {
								t.Fatal("original outgoing grant changed after rollback")
							}
						} else if restored := receivedRow(t, writer, original); !reflect.DeepEqual(restored, incomingBefore) {
							t.Fatal("original incoming grant changed after rollback")
						}
						// The new grant must be visible when the action selects again.
						later, writerErr = store(writer, token, nil)
						release()
					}
					var action result
					select {
					case action = <-done:
					case <-ctx.Done():
						t.Fatal("action did not complete")
					}
					if action.err != nil || writerErr != nil {
						t.Fatalf("action: %v, writer: %v", action.err, writerErr)
					}
					want := []string{original}
					if mode == "retry" {
						want = append(want, later)
					}
					if !slices.Equal(action.ids, want) {
						t.Fatalf("receipts %v, want %v", action.ids, want)
					}
					if mode == "commit" {
						later, writerErr = store(writer, token, laterID)
						if writerErr != nil {
							t.Fatalf("competing store after acknowledged commit: %v", writerErr)
						}
					}
					if direction == "outgoing" {
						var current []model.OcmShare
						if err := writer.db.Where("id IN ?", []string{original, later}).Find(&current).Error; err != nil {
							t.Fatal(err)
						}
						if mode == "commit" && (len(current) != 1 || current[0].Token != token) ||
							mode == "retry" && len(current) != 0 {
							t.Fatalf("unexpected live grants after %s: %d", mode, len(current))
						}
					} else {
						if first := receivedRow(t, writer, original); first.State != model.OcmShareStateRejected || !first.Hidden {
							t.Fatal("original incoming grant was not declined")
						}
						got, err := writer.GetReceivedShare(ctx, &userpb.User{Id: alice}, idRef(&ocm.ShareId{OpaqueId: later}))
						if err != nil {
							t.Fatal(err)
						}
						expected := ocm.ShareState_SHARE_STATE_PENDING
						if mode == "retry" {
							expected = ocm.ShareState_SHARE_STATE_REJECTED
						}
						if got.State != expected || got.Hidden != (mode == "retry") {
							t.Fatalf("later grant state %v, hidden %t, want state %v, hidden %t", got.State, got.Hidden, expected, mode == "retry")
						}
					}
				})
			}
		}
	})
}
