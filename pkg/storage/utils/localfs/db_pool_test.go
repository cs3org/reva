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

package localfs

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// An abandoned *sql.Rows keeps its connection until the process exits, so an
// uncapped pool turns a leak into a process-wide outage. This asserts the cap
// is actually applied: with it, only maxOpenConns result sets can be held
// open at once and the rest wait rather than piling up.
func TestPoolIsCapped(t *testing.T) {
	db, err := initializeDB(t.TempDir(), "pool.db")
	if err != nil {
		t.Fatalf("initializeDB: %v", err)
	}
	defer db.Close()

	if got := db.Stats().MaxOpenConnections; got != maxOpenConns {
		t.Fatalf("MaxOpenConnections = %d, want %d", got, maxOpenConns)
	}

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, "INSERT INTO metadata (resource, key, value) VALUES ('r', 'k', 'v')"); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Hold result sets without closing them, which is what the leak does.
	var held []interface{ Close() error }
	defer func() {
		for _, r := range held {
			r.Close()
		}
	}()

	for i := 0; i < maxOpenConns; i++ {
		rows, err := db.QueryContext(ctx, "SELECT key, value FROM metadata WHERE resource=?", "r")
		if err != nil {
			t.Fatalf("query %d: %v", i, err)
		}
		if !rows.Next() {
			t.Fatalf("query %d returned no rows", i)
		}
		held = append(held, rows)
	}

	if got := db.Stats().OpenConnections; got > maxOpenConns {
		t.Fatalf("OpenConnections = %d, want at most %d", got, maxOpenConns)
	}

	// The next query must block on the exhausted pool instead of opening a
	// connection number maxOpenConns+1.
	done := make(chan error, 1)
	go func() {
		rows, err := db.QueryContext(ctx, "SELECT key, value FROM metadata WHERE resource=?", "r")
		if err == nil {
			err = rows.Close()
		}
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("query past the cap completed instead of waiting: %v", err)
	case <-time.After(200 * time.Millisecond):
	}

	if got := db.Stats().OpenConnections; got > maxOpenConns {
		t.Fatalf("OpenConnections = %d while saturated, want at most %d", got, maxOpenConns)
	}
}

// The cap must not turn concurrent callers into failures. localfs serves
// parallel Stat calls, and the driver waits for a lock rather than erroring,
// so a small pool has to stay correct under contention.
func TestPoolUnderConcurrency(t *testing.T) {
	db, err := initializeDB(t.TempDir(), "conc.db")
	if err != nil {
		t.Fatalf("initializeDB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	for i := 0; i < 20; i++ {
		if _, err := db.ExecContext(ctx, "INSERT INTO metadata (resource, key, value) VALUES (?, ?, ?)",
			filepath.Base(t.TempDir()), "k", "v"); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rows, err := db.QueryContext(ctx, "SELECT key, value FROM metadata")
			if err != nil {
				errs <- err
				return
			}
			defer rows.Close()
			for rows.Next() {
				var k, v string
				if err := rows.Scan(&k, &v); err != nil {
					errs <- err
					return
				}
			}
			if err := rows.Err(); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent read failed: %v", err)
	}
	if got := db.Stats().OpenConnections; got > maxOpenConns {
		t.Errorf("OpenConnections = %d, want at most %d", got, maxOpenConns)
	}
}
