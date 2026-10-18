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
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// mysqlTestEnv names a disposable MySQL server for the OCM share tests, as
// "user:password@host:port". Without it only SQLite runs.
const mysqlTestEnv = "REVA_TEST_MYSQL"

type mysqlTarget struct {
	user, password, host string
	port                 int
}

func mysqlTestTarget(t *testing.T) (mysqlTarget, bool) {
	raw := os.Getenv(mysqlTestEnv)
	if raw == "" {
		return mysqlTarget{}, false
	}
	at := strings.LastIndex(raw, "@")
	if at < 0 {
		t.Fatalf("%s must be user:password@host:port", mysqlTestEnv)
	}
	creds, addr := raw[:at], raw[at+1:]
	colon := strings.Index(creds, ":")
	hostPort := strings.LastIndex(addr, ":")
	if colon < 0 || hostPort < 0 {
		t.Fatalf("%s must be user:password@host:port", mysqlTestEnv)
	}
	port, err := strconv.Atoi(addr[hostPort+1:])
	if err != nil {
		t.Fatalf("%s: invalid port", mysqlTestEnv)
	}
	return mysqlTarget{user: creds[:colon], password: creds[colon+1:], host: addr[:hostPort], port: port}, true
}

// openMySQLDatabase creates a fresh database for one test and drops it after.
func openMySQLDatabase(t *testing.T, target mysqlTarget) map[string]any {
	t.Helper()
	name := "ocm_t_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
	db, err := dbsql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%d)/", target.user, target.password, target.host, target.port))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec("CREATE DATABASE " + name + " CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec("DROP DATABASE " + name); err != nil {
			t.Errorf("drop disposable schema: %v", err)
		}
	})
	return map[string]any{
		"db_engine":   "mysql",
		"db_username": target.user,
		"db_password": target.password,
		"db_host":     target.host,
		"db_port":     target.port,
		"db_name":     name,
	}
}

// ocmTestConfig returns the manager configuration of a fresh database.
func ocmTestConfig(t *testing.T, dialect string) map[string]any {
	t.Helper()
	if dialect == "mysql" {
		target, _ := mysqlTestTarget(t)
		return openMySQLDatabase(t, target)
	}
	return map[string]any{
		"db_engine": "sqlite",
		"db_name":   filepath.Join(t.TempDir(), "ocm.sqlite"),
	}
}

// openOCMManager opens a manager, with its own connection pool, on cfg.
func openOCMManager(t *testing.T, cfg map[string]any) *mgr {
	t.Helper()
	repo, err := NewOCMShareManager(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	m := repo.(*mgr)
	pool, err := m.db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	return m
}

// forEachDialectConfig runs fn with the configuration of a fresh database on
// SQLite and, when configured, on MySQL.
func forEachDialectConfig(t *testing.T, fn func(t *testing.T, cfg map[string]any)) {
	dialects := []string{"sqlite"}
	if _, ok := mysqlTestTarget(t); ok {
		dialects = append(dialects, "mysql")
	}
	for _, d := range dialects {
		t.Run(d, func(t *testing.T) {
			fn(t, ocmTestConfig(t, d))
		})
	}
}

// forEachDialect runs fn against a fresh OCM share manager on SQLite and,
// when configured, on MySQL.
func forEachDialect(t *testing.T, fn func(t *testing.T, m *mgr)) {
	forEachDialectConfig(t, func(t *testing.T, cfg map[string]any) {
		fn(t, openOCMManager(t, cfg))
	})
}
