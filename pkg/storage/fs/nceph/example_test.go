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

//go:build ignore
// +build ignore

package main

import (
	"context"
	"fmt"
	"log"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/storage/fs/nceph"
)

func main() {
	// Configuration for the nceph filesystem
	config := map[string]interface{}{
		"root":             "/tmp/nceph_example",
		"user_layout":      "{{.Username}}",
		"dir_perms":        0755,
		"file_perms":       0644,
		"user_quota_bytes": 1000000000, // 1GB
	}

	ctx := context.Background()

	// Create a test user and add to context
	user := &userv1beta1.User{
		Id: &userv1beta1.UserId{
			OpaqueId: "example-user",
		},
		Username:    "alice",
		UidNumber:   1000,
		GidNumber:   1000,
		DisplayName: "Alice Example",
	}
	ctx = appctx.ContextSetUser(ctx, user)

	// Create the nceph filesystem instance
	fs, err := nceph.New(ctx, config)
	if err != nil {
		log.Fatalf("Failed to create nceph filesystem: %v", err)
	}

	// Create home directory
	if err := fs.CreateHome(ctx); err != nil {
		log.Fatalf("Failed to create home: %v", err)
	}

	home, err := fs.GetHome(ctx)
	if err != nil {
		log.Fatalf("Failed to get home: %v", err)
	}

	fmt.Printf("User home created at: %s\n", home)

	// Create a directory
	dirRef := &provider.Reference{Path: "documents"}
	if err := fs.CreateDir(ctx, dirRef); err != nil {
		log.Fatalf("Failed to create directory: %v", err)
	}

	// Touch a file
	fileRef := &provider.Reference{Path: "documents/example.txt"}
	if err := fs.TouchFile(ctx, fileRef); err != nil {
		log.Fatalf("Failed to touch file: %v", err)
	}

	// Get file metadata
	md, err := fs.GetMD(ctx, fileRef, nil)
	if err != nil {
		log.Fatalf("Failed to get metadata: %v", err)
	}

	fmt.Printf("File created: %s (type: %v)\n", md.Path, md.Type)

	// List folder contents
	homeRef := &provider.Reference{Path: "."}
	files, err := fs.ListFolder(ctx, homeRef, nil)
	if err != nil {
		log.Fatalf("Failed to list folder: %v", err)
	}

	fmt.Printf("Home directory contains %d items:\n", len(files))
	for _, file := range files {
		fmt.Printf("  - %s (%v)\n", file.Path, file.Type)
	}

	// Get quota information
	total, used, err := fs.GetQuota(ctx, homeRef)
	if err != nil {
		log.Fatalf("Failed to get quota: %v", err)
	}

	fmt.Printf("Quota: %d bytes used of %d bytes total\n", used, total)

	// Cleanup
	if err := fs.Shutdown(ctx); err != nil {
		log.Fatalf("Failed to shutdown: %v", err)
	}

	fmt.Println("NCeph filesystem example completed successfully!")
}
