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
	"math"
	"strings"

	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"github.com/cs3org/reva/v3/pkg/storage"
	eosclient "github.com/cs3org/reva/v3/pkg/storage/fs/eos/client"
	"github.com/pkg/errors"
)

const (
	// A group quota of eosclient.ProjectQuotaGID on the root of the space.
	quotaFormProject = "project"
	// The user quota of the owner on the quota node. Old spaces have it.
	quotaFormUser = "user"
)

var projectQuotaAuth = eosclient.Authorization{Role: eosclient.Role{UID: "0", GID: eosclient.ProjectQuotaGID}}

// GetSpaceQuota implements storage.QuotaAdmin.
func (fs *Eosfs) GetSpaceQuota(ctx context.Context, ref storage.SpaceQuotaRef) (*storage.SpaceQuota, error) {
	node, err := fs.spaceQuotaNode(ctx, ref)
	if err != nil {
		return nil, err
	}
	qi, err := fs.c.GetQuota(ctx, projectQuotaAuth, getSystemAuth(), node)
	if err != nil {
		return nil, errors.Wrapf(err, "eosfs: getting the quota of %s", node)
	}
	if qi.TotalBytes != 0 || ref.Owner == nil {
		return fs.spaceQuota(ref.Root, quotaFormProject, qi), nil
	}
	auth, err := fs.getUIDGateway(ctx, ref.Owner)
	if err != nil {
		return nil, errors.Wrapf(err, "eosfs: %s has no project quota, and the owner %s is unknown", node, ref.Owner.GetOpaqueId())
	}
	qi, err = fs.c.GetQuota(ctx, auth, getSystemAuth(), fs.conf.QuotaNode)
	if err != nil {
		return nil, errors.Wrapf(err, "eosfs: getting the quota of %s on %s", ref.Owner.GetOpaqueId(), fs.conf.QuotaNode)
	}
	return fs.spaceQuota(ref.Root, quotaFormUser, qi), nil
}

// SetSpaceQuota implements storage.QuotaAdmin. It always sets the project quota,
// also on a space with a user quota: EOS counts a file on the nearest quota
// node above it, so the root of the space wins.
func (fs *Eosfs) SetSpaceQuota(ctx context.Context, ref storage.SpaceQuotaRef, maxBytes, maxFiles uint64) (*storage.SpaceQuota, error) {
	node, err := fs.spaceQuotaNode(ctx, ref)
	if err != nil {
		return nil, err
	}
	if maxBytes > math.MaxUint64/fs.conf.QuotaReplicaFactor {
		return nil, errtypes.BadRequest(fmt.Sprintf("eosfs: %d bytes are too many", maxBytes))
	}
	// EOS sets a volume in physical bytes and reports it in logical bytes.
	info := &eosclient.SetQuotaInfo{QuotaNode: node, MaxBytes: maxBytes * fs.conf.QuotaReplicaFactor, MaxFiles: maxFiles}
	if err := fs.c.SetQuota(ctx, projectQuotaAuth, getSystemAuth(), info); err != nil {
		return nil, errors.Wrapf(err, "eosfs: setting the quota of %s", node)
	}
	qi, err := fs.c.GetQuota(ctx, projectQuotaAuth, getSystemAuth(), node)
	if err != nil {
		return nil, errors.Wrapf(err, "eosfs: the quota of %s is set, but reading it again failed", node)
	}
	if fs.quotaCache != nil {
		fs.quotaCache.set(node, qi)
	}
	return fs.spaceQuota(ref.Root, quotaFormProject, qi), nil
}

// ListSpaceQuotas implements storage.QuotaAdmin with the project quotas. It
// skips the quota nodes outside the namespace of this storage.
func (fs *Eosfs) ListSpaceQuotas(ctx context.Context) ([]*storage.SpaceQuota, error) {
	nodes, err := fs.c.ListAllQuota(ctx, getSystemAuth())
	if err != nil {
		return nil, errors.Wrap(err, "eosfs: listing the project quotas")
	}
	qs := make([]*storage.SpaceQuota, 0, len(nodes))
	for node, qi := range nodes {
		root, err := fs.unwrap(ctx, node)
		if err != nil || !strings.HasPrefix(root, "/") {
			continue
		}
		qs = append(qs, fs.spaceQuota(root, quotaFormProject, qi))
	}
	return qs, nil
}

func (fs *Eosfs) spaceQuotaNode(ctx context.Context, ref storage.SpaceQuotaRef) (string, error) {
	switch ref.Type {
	case spaces.SpaceTypeHome, spaces.SpaceTypeProject:
	default:
		return "", errtypes.BadRequest(fmt.Sprintf("eosfs: spaces of type %q have no quota", ref.Type))
	}
	if ref.Root == "" {
		return "", errtypes.BadRequest("eosfs: the root of the space is missing")
	}
	return fs.wrap(ctx, ref.Root), nil
}

func (fs *Eosfs) spaceQuota(root, form string, qi *eosclient.QuotaInfo) *storage.SpaceQuota {
	return &storage.SpaceQuota{
		Root:             root,
		MaxBytes:         qi.TotalBytes,
		UsedBytes:        qi.UsedBytes,
		MaxFiles:         qi.TotalInodes,
		UsedFiles:        qi.UsedInodes,
		PhysicalMaxBytes: qi.TotalBytes * fs.conf.QuotaReplicaFactor,
		Form:             form,
	}
}
