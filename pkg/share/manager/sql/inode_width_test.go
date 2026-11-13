// Copyright 2018-2025 CERN
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
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
)

// mysqlTestConfig builds a share manager config pointing at the MySQL/MariaDB
// instance described by the SQL_* environment variables used by the
// integration CI. It skips the test when they are not set.
func mysqlTestConfig(t *testing.T, dbName string) map[string]any {
	t.Helper()
	username := os.Getenv("SQL_USERNAME")
	password := os.Getenv("SQL_PASSWORD")
	address := os.Getenv("SQL_ADDRESS")
	if username == "" || password == "" || address == "" {
		t.Skip("SQL_USERNAME/SQL_PASSWORD/SQL_ADDRESS not set, skipping MySQL test")
	}
	host := address
	port := 3306
	if h, p, ok := strings.Cut(address, ":"); ok {
		host = h
		if n, err := strconv.Atoi(p); err == nil {
			port = n
		}
	}
	return map[string]any{
		"db_engine":   "mysql",
		"db_username": username,
		"db_password": password,
		"db_host":     host,
		"db_port":     port,
		"db_name":     dbName,
	}
}

// TestShareWithLongInode covers the localfs opaque id, which is
// "fileid-" + url.QueryEscape(path) and therefore easily longer than 32
// characters. The share manager stores it in the inode column, so on MySQL the
// column must hold it. With the old size:32 the insert failed with
// "Error 1406 (22001): Data too long for column 'inode'". SQLite does not
// enforce VARCHAR length, which is why the unit tests (sqlite) never caught it.
func TestShareWithLongInode(t *testing.T) {
	const dbName = "test_db_long_inode"
	cfg := mysqlTestConfig(t, dbName)

	ctx := context.Background()
	mgr, err := NewShareManager(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}

	// A unique owner per run so a leftover row from an earlier run does not
	// turn this into an "already exists" error.
	owner := fmt.Sprintf("owner%d", time.Now().UnixNano())
	userctx := getUserContext(owner)
	user, _ := appctx.ContextGetUser(userctx)

	// A realistic localfs opaque id: "fileid-" plus an URL-encoded path.
	opaqueID := "fileid-einstein%2Fshare-ci-1792481155737234595%2Fshared.txt"
	if len(opaqueID) <= 32 {
		t.Fatalf("test inode %q is not longer than 32 chars, it would not cover the bug", opaqueID)
	}

	file := &provider.ResourceInfo{
		Id: &provider.ResourceId{
			StorageId: "localhome",
			OpaqueId:  opaqueID,
		},
		Type:  provider.ResourceType_RESOURCE_TYPE_FILE,
		Path:  "/einstein/shared.txt",
		Owner: user.Id,
		Mtime: &typesv1beta1.Timestamp{Seconds: uint64(1700000000)},
	}

	share, err := mgr.Share(userctx, file, getUserShareGrant("1000", "file"))
	if err != nil {
		t.Fatalf("sharing a resource with a %d-char inode failed: %v", len(opaqueID), err)
	}

	// Read the share back, so the long id really round-trips through the DB
	// and not just through the in-memory object.
	got, err := mgr.GetShare(userctx, &collaboration.ShareReference{
		Spec: &collaboration.ShareReference_Id{Id: share.Id},
	})
	if err != nil {
		t.Fatalf("reading back the share with a long inode failed: %v", err)
	}
	if got.ResourceId == nil || got.ResourceId.OpaqueId != opaqueID {
		t.Fatalf("round-trip mismatch: got %+v, want opaque id %q", got.ResourceId, opaqueID)
	}
}
