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

//go:build !ceph
// +build !ceph

package cephfs

import (
	"context"

	"github.com/cs3org/reva/v3/pkg/storage"
	"github.com/cs3org/reva/v3/pkg/storage/fs/registry"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("cephfs", New)
}

// New returns an implementation to of the storage.FS interface that talk to
// a ceph filesystem.
func New(ctx context.Context, m map[string]interface{}) (storage.FS, error) {
	return nil, errors.New("cephfs: revad was compiled without CephFS support")
}
