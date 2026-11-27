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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/mattn/go-sqlite3"
)

func TestOCMDatabaseOptions(t *testing.T) {
	const required = "?_busy_timeout=10000&_txlock=immediate"
	for _, tc := range []struct {
		engine, input, want string
	}{
		{"sqlite", "", "file:" + required},
		{"sqlite", "ocm.sqlite", "ocm.sqlite" + required},
		{"sqlite", "ocm#share.sqlite?cache=private", "ocm#share.sqlite?cache=private&_busy_timeout=10000&_txlock=immediate"},
		{"sqlite", "file:ocm%23share.sqlite?mode=rwc&_txlock=IMMEDIATE#ignored", "file:ocm%23share.sqlite?mode=rwc&_txlock=IMMEDIATE&_busy_timeout=10000"},
		{"sqlite", "file:ocm.sqlite#ignored?mode=ro", "file:ocm.sqlite" + required},
		{"sqlite", "ocm.sqlite?_busy_timeout=10000&_timeout=10000", "ocm.sqlite?_busy_timeout=10000&_timeout=10000&_txlock=immediate"},
		{"sqlite", "file:ocm.sqlite?mode=rwc&modeof=mode%20source&other=a%2fb+%20&other=&%5Ftxlock=IMMEDIATE#fragment", "file:ocm.sqlite?mode=rwc&modeof=mode%20source&other=a%2fb+%20&other=&%5Ftxlock=IMMEDIATE&_busy_timeout=10000"},
		{"mysql", "ocm?x=1", "ocm?x=1"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			cfg := Config{}
			cfg.Engine, cfg.DBName = tc.engine, tc.input
			result, err := ocmDatabaseConfig(cfg)
			if err != nil || result.DBName != tc.want {
				t.Fatalf("database name = %q, err = %v, want %q", result.DBName, err, tc.want)
			}
		})
	}
}

func TestSQLiteOCMOpenOptions(t *testing.T) {
	const legacyFile = "?legacy/part #%20?mode=ro.sqlite"
	for _, tc := range []struct {
		name, dsn, file      string
		modeSource, existing bool
	}{
		{name: "temporary database"},
		{name: "escaped mode source", dsn: "file:mode-target.sqlite?mode=rwc&modeof=mode%20source#ignored", file: "mode-target.sqlite", modeSource: true},
		{name: "legacy leading question mark", dsn: legacyFile, file: legacyFile, existing: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			if tc.modeSource {
				if err := os.WriteFile("mode source", nil, 0700); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if tc.existing {
				if err := os.Mkdir(filepath.Dir(tc.file), 0700); err != nil {
					t.Fatal(err)
				}
				prior, err := dbsql.Open("sqlite3", tc.dsn)
				if err != nil {
					t.Fatal(err)
				}
				_, writeErr := prior.ExecContext(ctx, "CREATE TABLE legacy_marker AS SELECT 73 AS value")
				closeErr := prior.Close()
				if writeErr != nil || closeErr != nil {
					t.Fatalf("create marker: %v, close: %v", writeErr, closeErr)
				}
			}
			repo, err := NewOCMShareManager(ctx, map[string]any{"db_engine": "sqlite", "db_name": tc.dsn})
			if err != nil {
				t.Fatal(err)
			}
			pool, err := repo.(*mgr).db.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = pool.Close() })
			conn, err := pool.Conn(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()

			var file string
			if err := conn.QueryRowContext(ctx, "SELECT file FROM pragma_database_list WHERE name = 'main'").Scan(&file); err != nil {
				t.Fatal(err)
			}
			if tc.file == "" {
				if file != "" {
					t.Fatalf("temporary database opened a named file %q", file)
				}
				var journalMode string
				if err := conn.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil || journalMode != "delete" {
					t.Fatalf("temporary database journal mode = %q, err = %v", journalMode, err)
				}
				entries, err := os.ReadDir(".")
				if err != nil || len(entries) != 0 {
					t.Fatalf("temporary database left directory entries %v, err = %v", entries, err)
				}
			} else if !strings.HasSuffix(file, string(filepath.Separator)+tc.file) {
				t.Fatalf("main database file = %q, want %q", file, tc.file)
			}
			if tc.modeSource {
				source, err := os.Stat("mode source")
				if err != nil {
					t.Fatal(err)
				}
				destination, err := os.Stat(tc.file)
				if err != nil {
					t.Fatal(err)
				}
				if destination.Mode().Perm() != source.Mode().Perm() {
					t.Fatalf("modeof set %04o, want %04o", destination.Mode().Perm(), source.Mode().Perm())
				}
			}
			if tc.existing {
				var value int
				if err := conn.QueryRowContext(ctx, "SELECT value FROM legacy_marker").Scan(&value); err != nil || value != 73 {
					t.Fatalf("legacy marker = %d, err = %v", value, err)
				}
			}
			var busyTimeout int
			if err := conn.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busyTimeout); err != nil || busyTimeout != 10000 {
				t.Fatalf("busy timeout = %d ms, err = %v", busyTimeout, err)
			}

			// An immediate transaction reserves the attached file even
			// before any statement touches it.
			const probe = "write-reservation.sqlite"
			if _, err := conn.ExecContext(ctx, "ATTACH DATABASE ? AS probe", probe); err != nil {
				t.Fatal(err)
			}
			other, err := dbsql.Open("sqlite3", probe+"?_txlock=immediate&_busy_timeout=0")
			if err != nil {
				t.Fatal(err)
			}
			defer other.Close()
			holder, err := conn.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer holder.Rollback()
			competing, err := other.BeginTx(ctx, nil)
			if err == nil {
				_ = competing.Rollback()
				t.Fatal("competing writer acquired the lock before the manager transaction released it")
			}
			var sqliteErr sqlite3.Error
			if !errors.As(err, &sqliteErr) || sqliteErr.Code != sqlite3.ErrBusy {
				t.Fatalf("competing writer error = %v, want SQLITE_BUSY", err)
			}
			if err := holder.Rollback(); err != nil {
				t.Fatal(err)
			}
			competing, err = other.BeginTx(ctx, nil)
			if err != nil {
				t.Fatalf("competing writer failed after lock release: %v", err)
			}
			_ = competing.Rollback()
		})
	}
}

func TestSQLiteCompletionWaitsForCompetingWriter(t *testing.T) {
	cases := []struct {
		name     string
		filename string
		dsn      func(string) string
	}{
		{"plain path", "ocm#share.sqlite", func(path string) string { return path }},
		{"path with options", "ocm.sqlite", func(path string) string { return path + "?cache=private" }},
		{"file URI with options and fragment", "ocm#share.sqlite", func(path string) string {
			return "file:" + filepath.Join(filepath.Dir(path), "ocm%23share.sqlite") + "?mode=rwc#ignored?mode=ro"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, user := ownerContext(t, "marie")
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			path := filepath.Join(t.TempDir(), tc.filename)
			repo, err := NewOCMShareManager(ctx, map[string]any{"db_engine": "sqlite", "db_name": tc.dsn(path)})
			if err != nil {
				t.Fatal(err)
			}
			m := repo.(*mgr)
			pool, err := m.db.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = pool.Close() })
			if _, err := os.Stat(path); err != nil {
				t.Fatal(err)
			}

			rs := storeReceived(t, m, ctx, user, "contended")
			request := &ocm.ReceivedShare{Id: rs.Id, State: ocm.ShareState_SHARE_STATE_PENDING}
			start, err := m.StartReceivedShareTransfer(ctx, user, request, stateMask())
			if err != nil {
				t.Fatal(err)
			}

			other, err := dbsql.Open("sqlite3", path)
			if err != nil {
				t.Fatal(err)
			}
			defer other.Close()
			holder, err := other.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer holder.Rollback()
			if _, err := holder.ExecContext(ctx, "UPDATE ocm_received_shares SET mtime = mtime + 1"); err != nil {
				t.Fatal(err)
			}
			released := make(chan error, 1)
			go func() {
				time.Sleep(150 * time.Millisecond)
				released <- holder.Commit()
			}()
			accepted := ocm.ShareState_SHARE_STATE_ACCEPTED
			u, err := m.CompleteReceivedShareTransfer(ctx, user, rs.Id, start.Generation, accepted)
			if releaseErr := <-released; releaseErr != nil {
				t.Fatal(releaseErr)
			}
			if err != nil || !u.StateChanged() || u.After.GetState() != accepted {
				t.Fatalf("competing completion: %v %v", u, err)
			}

			if _, err := m.UpdateReceivedShare(ctx, user, request, stateMask()); err != nil {
				t.Fatal(err)
			}
			second, err := m.StartReceivedShareTransfer(ctx, user, request, stateMask())
			if err != nil {
				t.Fatal(err)
			}
			stale, err := m.CompleteReceivedShareTransfer(ctx, user, rs.Id, start.Generation, accepted)
			if err != nil || stale == nil || stale.Before == nil || stale.After == nil || stale.StateChanged() ||
				stale.After.GetState() != ocm.ShareState_SHARE_STATE_TRANSFERRING ||
				second.Generation <= start.Generation || receivedGeneration(t, m, rs.Id) != second.Generation {
				t.Fatalf("stale completion changed the new attempt: %v %v", stale, err)
			}
		})
	}
}

func TestSQLiteRejectsConflictingOCMOptions(t *testing.T) {
	for _, raw := range []string{
		"_txlock=deferred",
		"_busy_timeout=5",
		"_timeout=1",
		"_txlock=immediate&_txlock=exclusive",
		"_busy_timeout=10000&_busy_timeout=5",
		"_timeout=10000&_timeout=1",
		"a=%zz",
		"a=1;b=2",
	} {
		t.Run(raw, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ocm.sqlite")
			_, err := NewOCMShareManager(context.Background(), map[string]any{
				"db_engine": "sqlite", "db_name": path + "?" + raw,
			})
			if err == nil {
				t.Fatal("conflicting or malformed SQLite options were accepted")
			}
			if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
				t.Fatalf("configuration should be rejected before opening the database: %v", statErr)
			}
		})
	}
}
