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

package eos

import (
	"context"
	"fmt"

	"github.com/cs3org/reva/v3/pkg/storage"
	eosclient "github.com/cs3org/reva/v3/pkg/storage/fs/eos/client"
	"github.com/pkg/errors"
)

// Eosfs implements storage.QuotaAdmin: the operator surface the Admin API
// uses to read and set quotas, the way external management tools did with
// the EOS client directly.

// GetQuotaFor reads the quota of a node for the identity of q.
func (fs *Eosfs) GetQuotaFor(ctx context.Context, q storage.QuotaRef) (*storage.QuotaInfo, error) {
	auth, node, err := fs.quotaTarget(ctx, q)
	if err != nil {
		return nil, err
	}
	qi, err := fs.c.GetQuota(ctx, auth, getSystemAuth(), node)
	if err != nil {
		return nil, errors.Wrapf(err, "eosfs: getting the quota of %s", node)
	}
	return &storage.QuotaInfo{TotalBytes: qi.TotalBytes, UsedBytes: qi.UsedBytes, TotalInodes: qi.TotalInodes, UsedInodes: qi.UsedInodes}, nil
}

// SetQuotaFor sets the quota of a node for the identity of q, then refreshes
// the cached value users read through GetQuota.
func (fs *Eosfs) SetQuotaFor(ctx context.Context, q storage.QuotaRef, maxBytes, maxInodes uint64) error {
	auth, node, err := fs.quotaTarget(ctx, q)
	if err != nil {
		return err
	}
	info := &eosclient.SetQuotaInfo{UID: auth.Role.UID, GID: auth.Role.GID, QuotaNode: node, MaxBytes: maxBytes, MaxFiles: maxInodes}
	if err := fs.c.SetQuota(ctx, auth, getSystemAuth(), info); err != nil {
		return errors.Wrapf(err, "eosfs: setting the quota of %s", node)
	}
	if fs.quotaCache != nil {
		if qi, err := fs.c.GetQuota(ctx, auth, getSystemAuth(), node); err == nil {
			fs.quotaCache.set(node, qi)
		}
	}
	return nil
}

// quotaTarget maps a QuotaRef onto what the EOS clients expect: the role
// selecting the quota (a project quota is a group quota of ProjectQuotaGID and
// the clients want a numeric uid next to it) and the quota node, which for a
// user quota defaults to the storage's quota node.
func (fs *Eosfs) quotaTarget(ctx context.Context, q storage.QuotaRef) (eosclient.Authorization, string, error) {
	var auth eosclient.Authorization
	switch {
	case q.GID == eosclient.ProjectQuotaGID:
		auth = eosclient.Authorization{Role: eosclient.Role{UID: "0", GID: eosclient.ProjectQuotaGID}}
		if q.Path == "" {
			return auth, "", fmt.Errorf("eosfs: a project quota needs the path of the space")
		}
	case q.UID != "" && q.UID != "0":
		gid := q.GID
		if gid == "" {
			gid = "0"
		}
		auth = eosclient.Authorization{Role: eosclient.Role{UID: q.UID, GID: gid}}
	default:
		return auth, "", fmt.Errorf("eosfs: a quota needs a uid, or the project quota gid %s", eosclient.ProjectQuotaGID)
	}
	node := fs.conf.QuotaNode
	if q.Path != "" && q.Path != "/" {
		node = fs.wrap(ctx, q.Path)
	}
	return auth, node, nil
}
