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

package storageprovider

import (
	"cmp"
	"context"
	"fmt"
	"path"
	"strconv"
	"strings"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"github.com/cs3org/reva/v3/pkg/storage"
)

type spaceQuotaArgs struct {
	Root      string           `arg:"root,required"       help:"the root path of the space"`
	SpaceType spaces.SpaceType `arg:"space_type"          help:"the type of the space, e.g. personal or project; if absent, the type of the spaces of this instance"`
	Owner     string           `arg:"owner"               help:"the user id of the owner of the space"`
}

type setSpaceQuotaArgs struct {
	Root      string           `arg:"root,required"       help:"the root path of the space"`
	SpaceType spaces.SpaceType `arg:"space_type"          help:"the type of the space, e.g. personal or project; if absent, the type of the spaces of this instance"`
	Owner     string           `arg:"owner"               help:"the user id of the owner of the space"`
	MaxBytes  *uint64          `arg:"max_bytes"           help:"the quota in logical bytes, the size of the data users see; if absent, it does not change"`
	MaxFiles  *uint64          `arg:"max_files"           help:"the quota in files; if absent, it does not change"`
}

type listSpaceQuotasArgs struct {
	MinBytes uint64 `arg:"min_bytes" help:"only the quotas of at least this many logical bytes"`
}

func (s *service) addQuotaInvocations() {
	s.Add("quota_get", "Show the quota of a space that this instance serves.").
		Handle(invoke.Typed(s.quotaGet))
	s.Add("quota_set", "Set the quota of a space that this instance serves.").
		Mutating().
		Handle(invoke.Typed(s.quotaSet))
	s.Add("quota_list", "List the quotas that the storage of this instance keeps for spaces.").
		HandleStream(invoke.TypedStream(s.quotaList))
}

func (s *service) quotaGet(ctx context.Context, a spaceQuotaArgs) (invoke.Result, error) {
	qa, ref, err := s.spaceQuotaRef(a.Root, a.SpaceType, a.Owner)
	if err != nil {
		return nil, err
	}
	q, err := qa.GetSpaceQuota(ctx, ref)
	if err != nil {
		return nil, err
	}
	return s.quotaResult(q, ref.Type, a.Owner), nil
}

func (s *service) quotaSet(ctx context.Context, a setSpaceQuotaArgs) (invoke.Result, error) {
	if a.MaxBytes == nil && a.MaxFiles == nil {
		return nil, errtypes.BadRequest("give max_bytes, max_files or both")
	}
	qa, ref, err := s.spaceQuotaRef(a.Root, a.SpaceType, a.Owner)
	if err != nil {
		return nil, err
	}
	old, err := qa.GetSpaceQuota(ctx, ref)
	if err != nil {
		return nil, err
	}
	maxBytes, maxFiles := old.MaxBytes, old.MaxFiles
	if a.MaxBytes != nil {
		maxBytes = *a.MaxBytes
	}
	if a.MaxFiles != nil {
		maxFiles = *a.MaxFiles
	}
	q, err := qa.SetSpaceQuota(ctx, ref, maxBytes, maxFiles)
	u, _ := appctx.ContextGetUser(ctx)
	admin.Audit(ctx, admin.AuditEvent{
		Action:  "storageprovider.quota_set",
		Actor:   u.GetUsername(),
		Target:  a.Root,
		Kind:    string(invoke.KindMutating),
		Granted: err == nil,
		Err:     err,
		Fields: map[string]string{
			"mount_id":      s.mountID,
			"space_type":    string(ref.Type),
			"old_max_bytes": strconv.FormatUint(old.MaxBytes, 10),
			"old_max_files": strconv.FormatUint(old.MaxFiles, 10),
			"max_bytes":     strconv.FormatUint(maxBytes, 10),
			"max_files":     strconv.FormatUint(maxFiles, 10),
		},
	})
	if err != nil {
		return nil, err
	}
	return s.quotaResult(q, ref.Type, a.Owner), nil
}

func (s *service) quotaList(ctx context.Context, a listSpaceQuotasArgs, emit invoke.StreamEmit) error {
	qa, ok := s.storage.(storage.QuotaAdmin)
	if !ok {
		return errtypes.NotSupported("the " + s.conf.Driver + " driver cannot administer quotas")
	}
	qs, err := qa.ListSpaceQuotas(ctx)
	if err != nil {
		return err
	}
	for _, q := range qs {
		if q.MaxBytes < a.MinBytes {
			continue
		}
		if err := emit(s.quotaResult(q, "", "")); err != nil {
			return err
		}
	}
	return nil
}

// spaceQuotaRef refuses a root that is not the root of a space served here,
// so that a quota never goes on a parent of many spaces.
func (s *service) spaceQuotaRef(root string, spaceType spaces.SpaceType, owner string) (storage.QuotaAdmin, storage.SpaceQuotaRef, error) {
	qa, ok := s.storage.(storage.QuotaAdmin)
	if !ok {
		return nil, storage.SpaceQuotaRef{}, errtypes.NotSupported("the " + s.conf.Driver + " driver cannot administer quotas")
	}
	if !strings.HasPrefix(root, "/") {
		return nil, storage.SpaceQuotaRef{}, errtypes.BadRequest(fmt.Sprintf("root must be an absolute path, not %q", root))
	}
	root, mount := path.Clean(root), path.Clean(s.mountPath)
	if root != mount && !strings.HasPrefix(root, strings.TrimSuffix(mount, "/")+"/") {
		return nil, storage.SpaceQuotaRef{}, errtypes.BadRequest(fmt.Sprintf("%s is not under the mount %s of this instance", root, mount))
	}
	single := s.conf.SpaceDepth <= pathLevels(mount)
	if single && root != mount || !single && pathLevels(root) != s.conf.SpaceDepth {
		return nil, storage.SpaceQuotaRef{}, errtypes.BadRequest(fmt.Sprintf("%s is not the root of a space of this instance", root))
	}
	rel, err := s.trimMountPrefix(root)
	if err != nil {
		return nil, storage.SpaceQuotaRef{}, err
	}
	ref := storage.SpaceQuotaRef{Root: rel, Type: cmp.Or(spaceType, spaces.SpaceType(s.conf.ProvidesSpaceType))}
	if owner != "" {
		ref.Owner = &userpb.UserId{OpaqueId: owner}
	}
	return qa, ref, nil
}

func (s *service) quotaResult(q *storage.SpaceQuota, spaceType spaces.SpaceType, owner string) invoke.Result {
	r := invoke.Result{
		"root":               path.Join(s.mountPath, q.Root),
		"mount_id":           s.mountID,
		"max_bytes":          q.MaxBytes,
		"used_bytes":         q.UsedBytes,
		"max_files":          q.MaxFiles,
		"used_files":         q.UsedFiles,
		"physical_max_bytes": q.PhysicalMaxBytes,
		"form":               q.Form,
	}
	if spaceType != "" {
		r["space_type"] = string(spaceType)
	}
	if owner != "" {
		r["owner"] = owner
	}
	return r
}
