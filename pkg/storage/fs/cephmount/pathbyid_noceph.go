//go:build !ceph

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

// GetPathByID implementation without ceph support
package cephmount

import (
	"context"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
)

// CephAdminConn represents the admin connection to ceph for GetPathByID operations
// This is a stub when ceph support is disabled
type CephAdminConn struct{}

// newCephAdminConn creates a ceph admin connection for GetPathByID operations
// This always returns an error when ceph support is disabled
func newCephAdminConn(ctx context.Context, conf *Options) (*CephAdminConn, error) {
	return nil, errtypes.NotSupported("cephmount: ceph support not enabled (build with -tags ceph)")
}

// Close is a no-op when ceph support is disabled
func (c *CephAdminConn) Close() {
	// no-op
}

func (fs *cephmountfs) GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error) {
	return "", errtypes.NotSupported("cephmount: GetPathByID requires ceph support (build with -tags ceph)")
}
