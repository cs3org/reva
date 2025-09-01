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

package nceph

import (
	"context"
	"os"
	"testing"
	"time"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestDebugLogging(t *testing.T) {
	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "nceph_debug_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Set permissions for test user
	err = os.Chmod(tmpDir, 0755)
	require.NoError(t, err)
	err = os.Chown(tmpDir, 1000, 1000)
	require.NoError(t, err)

	// Create context with debug logging enabled
	ctx := context.Background()
	
	// Set up debug logger 
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}).
		Level(zerolog.DebugLevel).With().Timestamp().Logger()
	ctx = appctx.WithLogger(ctx, &logger)

	// Add a test user to the context
	user := &userv1beta1.User{
		Id: &userv1beta1.UserId{
			OpaqueId: "debug-test-user",
		},
		Username:  "debuguser",
		UidNumber: 1000,
		GidNumber: 1000,
	}
	ctx = appctx.ContextSetUser(ctx, user)

	// Set environment variable to use tmpDir as chroot
	originalChrootDir := os.Getenv("NCEPH_TEST_CHROOT_DIR")
	os.Setenv("NCEPH_TEST_CHROOT_DIR", tmpDir)
	defer func() {
		if originalChrootDir == "" {
			os.Unsetenv("NCEPH_TEST_CHROOT_DIR")
		} else {
			os.Setenv("NCEPH_TEST_CHROOT_DIR", originalChrootDir)
		}
	}()

	// Create nceph instance
	config := map[string]interface{}{
		"allow_local_mode": true,
	}

	fs, err := New(ctx, config)
	require.NoError(t, err)
	require.NotNil(t, fs)
	defer fs.Shutdown(ctx)

	t.Log("=== Debug Logging Test ===")
	t.Log("Look for debug log entries showing: operation, path, username, uid, thread_id")
	
	// Test CreateDir
	ref := &provider.Reference{Path: "/testdir"}
	err = fs.CreateDir(ctx, ref)
	require.NoError(t, err)

	// Test GetMD
	_, err = fs.GetMD(ctx, ref, []string{})
	require.NoError(t, err)

	// Test TouchFile
	fileRef := &provider.Reference{Path: "/testfile.txt"}
	err = fs.TouchFile(ctx, fileRef)
	require.NoError(t, err)

	// Test ListFolder
	_, err = fs.ListFolder(ctx, &provider.Reference{Path: "/"}, []string{})
	require.NoError(t, err)

	t.Log("Debug logging test completed - check output for structured log entries")
}
