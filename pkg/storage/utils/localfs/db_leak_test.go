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

package localfs

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"path"
	"sync"
	"testing"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	sqlite "github.com/glebarez/go-sqlite"
)

// countingDriver wraps the real sqlite driver and counts Prepare against
// Close. A localfs method that calls db.Prepare without closing the returned
// statement leaks one prepared statement per call. On MariaDB that eventually
// exhausts max_prepared_stmt_count (16382) and then every request fails with
// "Can't create more than max_prepared_stmt_count statements", because the
// home-directory existence check can no longer stat the user's space. The
// symptom is a 401 on every DAV/graph call even though authentication
// succeeded.
type countingDriver struct {
	inner driver.Driver

	mu       sync.Mutex
	prepared int
	closed   int
	queried  int
	rowsDone int
}

func (d *countingDriver) Open(name string) (driver.Conn, error) {
	c, err := d.inner.Open(name)
	if err != nil {
		return nil, err
	}
	return &countingConn{Conn: c, d: d}, nil
}

type countingConn struct {
	driver.Conn
	d *countingDriver
}

func (c *countingConn) Query(q string, args []driver.Value) (driver.Rows, error) {
	qc, ok := c.Conn.(driver.Queryer)
	if !ok {
		return nil, driver.ErrSkip
	}
	r, err := qc.Query(q, args)
	if err != nil {
		return nil, err
	}
	c.d.mu.Lock()
	c.d.queried++
	c.d.mu.Unlock()
	return &countingRows{Rows: r, d: c.d}, nil
}

type countingRows struct {
	driver.Rows
	d *countingDriver
}

func (r *countingRows) Close() error {
	r.d.mu.Lock()
	r.d.rowsDone++
	r.d.mu.Unlock()
	return r.Rows.Close()
}

func (c *countingConn) Prepare(q string) (driver.Stmt, error) {
	s, err := c.Conn.Prepare(q)
	if err != nil {
		return nil, err
	}
	c.d.mu.Lock()
	c.d.prepared++
	c.d.mu.Unlock()
	return &countingStmt{Stmt: s, d: c.d}, nil
}

type countingStmt struct {
	driver.Stmt
	d *countingDriver
}

func (s *countingStmt) Close() error {
	s.d.mu.Lock()
	s.d.closed++
	s.d.mu.Unlock()
	return s.Stmt.Close()
}

// TestPreparedStatementsAreClosed drives the localfs DB helpers and asserts
// that every prepared statement is released.
func TestPreparedStatementsAreClosed(t *testing.T) {
	d := &countingDriver{inner: &sqlite.Driver{}}
	sql.Register("counting-sqlite3", d)

	db, err := sql.Open("counting-sqlite3", path.Join(t.TempDir(), "leak.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for _, stmt := range []string{
		"CREATE TABLE IF NOT EXISTS recycled_entries (key TEXT PRIMARY KEY, path TEXT)",
		"CREATE TABLE IF NOT EXISTS user_interaction (resource TEXT, grantee TEXT, role TEXT DEFAULT '', favorite INTEGER DEFAULT 0, PRIMARY KEY (resource, grantee))",
		"CREATE TABLE IF NOT EXISTS metadata (resource TEXT, key TEXT, value TEXT, PRIMARY KEY (resource, key))",
		"CREATE TABLE IF NOT EXISTS share_references (resource TEXT PRIMARY KEY, target TEXT)",
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}

	fs := &localfs{db: db}
	ctx := context.Background()

	const n = 25
	for i := 0; i < n; i++ {
		k := fmt.Sprintf("k%d", i)
		res := fmt.Sprintf("res%d", i)
		if err := fs.addToRecycledDB(ctx, k, "file"); err != nil {
			t.Fatal(err)
		}
		if err := fs.removeFromRecycledDB(ctx, k); err != nil {
			t.Fatal(err)
		}
		if err := fs.addToACLDB(ctx, res, "einstein", "owner"); err != nil {
			t.Fatal(err)
		}
		if err := fs.removeFromACLDB(ctx, res, "einstein"); err != nil {
			t.Fatal(err)
		}
		if err := fs.addToFavoritesDB(ctx, res, "einstein"); err != nil {
			t.Fatal(err)
		}
		if err := fs.removeFromFavoritesDB(ctx, res, "einstein"); err != nil {
			t.Fatal(err)
		}
		if err := fs.addToMetadataDB(ctx, res, "key", "value"); err != nil {
			t.Fatal(err)
		}
		if err := fs.removeFromMetadataDB(ctx, res, "key"); err != nil {
			t.Fatal(err)
		}
		if err := fs.addToReferencesDB(ctx, res, "target"); err != nil {
			t.Fatal(err)
		}
		if err := fs.copyMD(res, res+"-copy"); err != nil {
			t.Fatal(err)
		}
		rows, err := fs.getMetadata(ctx, res)
		if err != nil {
			t.Fatal(err)
		}
		_ = rows.Close()
		acls, err := fs.getACLs(ctx, res)
		if err != nil {
			t.Fatal(err)
		}
		_ = acls.Close()
	}

	d.mu.Lock()
	prepared, closed := d.prepared, d.closed
	queried, rowsDone := d.queried, d.rowsDone
	d.mu.Unlock()

	if queried != rowsDone {
		t.Fatalf("result sets leaked: %d queried, %d closed (%d still open)", queried, rowsDone, queried-rowsDone)
	}

	if prepared != closed {
		t.Fatalf("prepared statements leaked: %d prepared, %d closed (%d still open)",
			prepared, closed, prepared-closed)
	}
	if prepared == 0 {
		t.Fatal("no prepared statements observed; the test did not exercise the DB path")
	}
}

// TestResultSetsAreClosed covers the other half of the leak: helpers that
// return *sql.Rows to their caller. Closing the statement is not enough,
// because an open result set keeps the statement alive on the server. These
// two paths are reached on every Stat (metadata) and every ListGrants call,
// so a leak here grows with normal browsing traffic just like the statement
// leak did.
func TestResultSetsAreClosed(t *testing.T) {
	d := &countingDriver{inner: &sqlite.Driver{}}
	sql.Register("counting-sqlite3-rows", d)

	db, err := sql.Open("counting-sqlite3-rows", path.Join(t.TempDir(), "rows.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for _, stmt := range []string{
		"CREATE TABLE IF NOT EXISTS user_interaction (resource TEXT, grantee TEXT, role TEXT DEFAULT '', favorite INTEGER DEFAULT 0, PRIMARY KEY (resource, grantee))",
		"CREATE TABLE IF NOT EXISTS metadata (resource TEXT, key TEXT, value TEXT, PRIMARY KEY (resource, key))",
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}

	fs := &localfs{db: db, conf: &Config{DisableHome: true, DataDirectory: t.TempDir()}}
	ctx := context.Background()

	for i := 0; i < 25; i++ {
		res := fmt.Sprintf("/res%d", i)
		if err := fs.addToMetadataDB(ctx, res, "key", "value"); err != nil {
			t.Fatal(err)
		}
		if err := fs.addToACLDB(ctx, res, "einstein", "owner"); err != nil {
			t.Fatal(err)
		}

		// retrieveArbitraryMetadata owns the rows it opens and must close them.
		md, err := fs.retrieveArbitraryMetadata(ctx, res, nil)
		if err != nil {
			t.Fatal(err)
		}
		if md.Metadata["key"] != "value" {
			t.Fatalf("metadata not read back for %s: %v", res, md.Metadata)
		}

		// ListGrants does the same for the ACL result set.
		if _, err := fs.ListGrants(ctx, &provider.Reference{Path: res}); err != nil {
			t.Fatal(err)
		}
	}

	// Full iteration closes the rows implicitly, so the case that actually
	// leaks is the early return: a Scan failure abandons the result set and
	// only a deferred Close releases it. A NULL in a string column fails
	// Scan, which is the same path a real DB error takes.
	if _, err := db.Exec("INSERT INTO metadata (resource, key, value) VALUES ('/null', 'k', NULL)"); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.retrieveArbitraryMetadata(ctx, "/null", nil); err == nil {
		t.Fatal("expected a scan error for the NULL value")
	}
	d.mu.Lock()
	queried, rowsDone := d.queried, d.rowsDone
	d.mu.Unlock()

	if queried != rowsDone {
		t.Fatalf("result sets leaked: %d queried, %d closed (%d still open)",
			queried, rowsDone, queried-rowsDone)
	}
	if queried == 0 {
		t.Fatal("no result sets observed; the test did not exercise the DB path")
	}
}
