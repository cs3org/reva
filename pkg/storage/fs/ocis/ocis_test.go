// Copyright 2018-2021 CERN
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

// +build storageRace

package ocis

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"sync"
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/user"
	"github.com/stretchr/testify/assert"
)

// TestLackAdvisoryLocks demonstrates that access to a file
// is not mutually exclusive on the oCIS storage.
var (
	config = make(map[string]interface{})
	ctx    context.Context
	f, f1  *os.File
	tmpDir string
)

func TestMain(m *testing.M) {
	var err error

	// prepare storage
	{
		tmpDir, _ = ioutil.TempDir("", "ocis_fs_unittests")
		{
			config["root"] = tmpDir
			config["enable_home"] = false
			config["user_layout"] = "{{.Id.OpaqueId}}"
			config["owner"] = "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c"
		}
	}

	// prepare context
	{
		u := &userpb.User{
			Id: &userpb.UserId{
				OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
			},
			Username:    "test",
			Mail:        "marie@example.org",
			DisplayName: "Marie Curie",
			Groups: []string{
				"radium-lovers",
				"polonium-lovers",
				"physics-lovers",
			},
		}
		ctx = user.ContextSetUser(context.Background(), u)
	}

	// do not do this. Prepare f0
	if err = ioutil.WriteFile(fmt.Sprintf("%s/%s", tmpDir, "f.lol"), []byte("test"), 0644); err != nil {
		panic(err)
	}
	f, err = os.Open(fmt.Sprintf("%s/%s", tmpDir, "f.lol"))
	if err != nil {
		panic(err)
	}

	// do not do this. Prepare f1
	if err = ioutil.WriteFile(fmt.Sprintf("%s/%s", tmpDir, "f1.lol"), []byte("another run"), 0644); err != nil {
		panic(err)
	}
	f1, err = os.Open(fmt.Sprintf("%s/%s", tmpDir, "f1.lol"))
	if err != nil {
		panic(err)
	}

	fmt.Printf("%s\n", tmpDir)
	m.Run()

	cts, err := ioutil.ReadFile(path.Join(tmpDir, "nodes", "root", "uploaded.txt"))
	if err != nil {
		panic(err)
	}
	fmt.Println(string(cts))
}

// Scenario: start 2 uploads, pause the first one, let the second one finish first,
// resume the first one at some point in time. Both uploads should finish.
// Needs to result in 2 versions, last finished is the most recent version.
func TestTwoUploadsVersioning(t *testing.T) {
	//runtime.GOMAXPROCS(1) // uncomment to remove concurrency and see revisions working.
	ofs, err := New(config)
	if err != nil {
		t.Error(err)
	}

	wg := &sync.WaitGroup{}
	wg.Add(2)

	// upload file with contents: "test"
	go func(wg *sync.WaitGroup) {
		ofs.Upload(ctx, &provider.Reference{
			Spec: &provider.Reference_Path{Path: "uploaded.txt"},
		}, f)
		wg.Done()
	}(wg)

	// upload file with contents: "another run"
	go func(wg *sync.WaitGroup) {
		ofs.Upload(ctx, &provider.Reference{
			Spec: &provider.Reference_Path{Path: "uploaded.txt"},
		}, f1)
		wg.Done()
	}(wg)

	// this test, by the way the oCIS storage is implemented, is non-deterministic, and the contents
	// of uploaded.txt will change on each run depending on which of the 2 routines above makes it
	// first into the scheduler. In order to make it deterministic, we have to consider the Upload impl-
	// ementation and we can leverage concurrency and add locks only when the destination path are the
	// same for 2 uploads.

	wg.Wait()
	revisions, err := ofs.ListRevisions(ctx, &provider.Reference{
		Spec: &provider.Reference_Path{Path: "uploaded.txt"},
	})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(revisions))
}

// TestParallelMkcol ensures that, on an unit level, if multiple requests fight for creating a directory (race condition)
// only the first one will create it. Note that there is little to synchronize here because if the folder is already
// created, the underlying filesystem (not the storage driver layer) will fail when attempting to create the directory.
func TestParallelMkcol(t *testing.T) {
	ofs, err := New(config)
	if err != nil {
		t.Error(err)
	}

	for i := 0; i < 10; i++ {
		t.Run("", func(t *testing.T) {
			t.Parallel()
			if err := ofs.CreateDir(ctx, "fightforit"); err != nil {
				rinfo, err := ofs.GetMD(ctx, &provider.Reference{
					Spec: &provider.Reference_Path{Path: "fightforit"},
				}, nil)
				if err != nil {
					t.Error(err)
				}

				assert.NotNil(t, rinfo)
			}
		})
	}
}
