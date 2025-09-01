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
	"fmt"
	"io"
	"log"
	"strings"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
)

// ExampleNew demonstrates basic usage of the nceph filesystem
func ExampleNew() {
	// Configuration example - only fstab entry needed
	config := map[string]interface{}{
		"fstabentry":       "user@cluster:/ /mnt/cephfs ceph defaults",
		"allow_local_mode": true, // For testing
	}

	ctx := context.Background()

	// Create a test user
	user := &userv1beta1.User{
		Id: &userv1beta1.UserId{
			OpaqueId: "example-user",
		},
		Username:  "alice",
		UidNumber: 1000,
		GidNumber: 1000,
	}
	ctx = appctx.ContextSetUser(ctx, user)

	// Create the filesystem
	fs, err := New(ctx, config)
	if err != nil {
		log.Fatalf("Failed to create filesystem: %v", err)
	}

	// Example operations
	ref := &provider.Reference{Path: "/example.txt"}

	// Upload a file
	content := strings.NewReader("Hello from nceph!")
	err = fs.Upload(ctx, ref, io.NopCloser(content), nil)
	if err != nil {
		log.Printf("Upload failed: %v", err)
	}

	// Get metadata
	md, err := fs.GetMD(ctx, ref, nil)
	if err != nil {
		log.Printf("GetMD failed: %v", err)
	} else {
		fmt.Printf("File: %s (size: %d bytes)\n", md.Path, md.Size)
	}

	// List directory
	dirRef := &provider.Reference{Path: "/"}
	files, err := fs.ListFolder(ctx, dirRef, nil)
	if err != nil {
		log.Printf("ListFolder failed: %v", err)
	} else {
		fmt.Printf("Directory contains %d items\n", len(files))
	}
}
