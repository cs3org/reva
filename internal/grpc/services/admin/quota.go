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

package admin

import (
	"context"
	"maps"
	"path"
	"slices"
	"strconv"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/invoke/client"
	"github.com/cs3org/reva/v3/pkg/registry"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// quotaSpace is a space as the quota invocations of the storage provider
// take it. An empty spaceType lets the storage provider use the type of its
// spaces.
type quotaSpace struct {
	id                     *provider.StorageSpaceId
	root, spaceType, owner string
}

func (a *spacesAdmin) GetSpaceQuota(ctx context.Context, req *adminpb.GetSpaceQuotaRequest) (*adminpb.SpaceQuota, error) {
	sp, eps, err := a.quotaTarget(ctx, req.Ref)
	if err != nil {
		return nil, err
	}
	return invokeQuota(ctx, eps, "quota_get", sp, sp.args())
}

func (a *spacesAdmin) SetSpaceQuota(ctx context.Context, req *adminpb.SetSpaceQuotaRequest) (*adminpb.SpaceQuota, error) {
	if req.MaxBytes == nil && req.MaxFiles == nil {
		return nil, status.Error(codes.InvalidArgument, "admin: set needs max_bytes, max_files or both")
	}
	sp, eps, err := a.quotaTarget(ctx, req.Ref)
	if err != nil {
		return nil, err
	}
	args := sp.args()
	if req.MaxBytes != nil {
		args["max_bytes"] = strconv.FormatUint(*req.MaxBytes, 10)
	}
	if req.MaxFiles != nil {
		args["max_files"] = strconv.FormatUint(*req.MaxFiles, 10)
	}
	q, err := invokeQuota(ctx, eps, "quota_set", sp, args)
	admin.Audit(ctx, admin.AuditEvent{
		Action:  "spaces.set_quota",
		Actor:   actorName(ctx),
		Target:  "space:" + sp.root,
		Granted: err == nil,
		Err:     err,
		Fields:  args,
	})
	return q, err
}

// ListSpaceQuotas asks one instance of each mount: the instances of a mount
// share the storage, so each one lists the same quotas.
func (a *spacesAdmin) ListSpaceQuotas(req *adminpb.ListSpaceQuotasRequest, stream adminpb.SpacesAdmin_ListSpaceQuotasServer) error {
	ctx := stream.Context()
	reg, err := a.s.registryHandle()
	if err != nil {
		return err
	}
	sv, err := reg.GetService(storageProviderService)
	if err != nil {
		return status.Errorf(codes.Unavailable, "admin: %v", err)
	}
	byMount := map[string][]client.Endpoint{}
	for _, n := range sv.Nodes() {
		md := n.Metadata()
		if md[registry.MetaState] == registry.StateOffline || !slices.Contains(strings.Split(md[invoke.MetaInvocations], ","), "quota_list") {
			continue
		}
		key := md[registry.MetaMountID]
		if key == "" {
			key = n.ID()
		}
		byMount[key] = append(byMount[key], client.EndpointFor(n))
	}
	if len(byMount) == 0 {
		return status.Error(codes.Unimplemented, "admin: no storage provider can administer quotas")
	}
	var eps []client.Endpoint
	for _, k := range slices.Sorted(maps.Keys(byMount)) {
		eps = append(eps, client.Pick(ctx, byMount[k]))
	}
	args := map[string]string{}
	if req.MinBytes > 0 {
		args["min_bytes"] = strconv.FormatUint(req.MinBytes, 10)
	}
	return relayStream(ctx, eps, "quota_list", args, stream.Send)
}

// quotaTarget resolves ref to its space, and the space to the storage
// provider instances of the mount that serves its root.
func (a *spacesAdmin) quotaTarget(ctx context.Context, ref *adminpb.SpaceRef) (quotaSpace, []client.Endpoint, error) {
	sp, err := a.spaceForQuota(ctx, ref)
	if err != nil {
		return quotaSpace{}, nil, err
	}
	mounts, err := (&storageAdmin{s: a.s}).ResolvePath(ctx, &adminpb.ResolvePathRequest{Path: sp.root})
	if err != nil {
		return quotaSpace{}, nil, err
	}
	// The registry also gives the mounts below a path, so keep the deepest
	// mount that holds the root.
	var mount *adminpb.Mount
	depth := -1
	for _, m := range mounts.Mounts {
		mp := path.Clean(m.MountPath)
		if (sp.root == mp || strings.HasPrefix(sp.root, strings.TrimSuffix(mp, "/")+"/")) && len(mp) > depth {
			mount, depth = m, len(mp)
		}
	}
	if mount == nil {
		return quotaSpace{}, nil, status.Errorf(codes.NotFound, "admin: no mount serves %s", sp.root)
	}
	reg, err := a.s.registryHandle()
	if err != nil {
		return quotaSpace{}, nil, err
	}
	eps, err := client.ResolveMetadata(reg, storageProviderService, map[string]string{registry.MetaMountID: mount.MountId})
	if err != nil {
		return quotaSpace{}, nil, status.Errorf(codes.Unavailable, "admin: %v", err)
	}
	return sp, eps, nil
}

// spaceForQuota finds the space of ref in the catalogue. The catalogue has only
// project spaces, so a space that is not in it is known by its root: the path
// of ref, or the path that its id encodes. The storage provider refuses a
// path that is not the root of a space.
func (a *spacesAdmin) spaceForQuota(ctx context.Context, ref *adminpb.SpaceRef) (quotaSpace, error) {
	m, err := a.GetSpace(ctx, &adminpb.GetSpaceRequest{Ref: ref})
	if err == nil {
		s := m.GetSpace()
		if s.GetRootInfo().GetPath() == "" {
			return quotaSpace{}, status.Errorf(codes.FailedPrecondition, "admin: the space %s has no root path in the catalogue", s.GetId().GetOpaqueId())
		}
		return quotaSpace{
			id:        s.GetId(),
			root:      path.Clean(s.GetRootInfo().GetPath()),
			spaceType: s.GetSpaceType(),
			owner:     s.GetOwner().GetId().GetOpaqueId(),
		}, nil
	}
	if status.Code(err) != codes.NotFound {
		return quotaSpace{}, err
	}
	root := ref.GetPath()
	if id := ref.GetId().GetOpaqueId(); id != "" {
		if _, p, ok := spaces.DecodeStorageSpaceIDToPath(id); ok {
			root = p
		} else if p, derr := spaces.DecodeSpaceID(id); derr == nil {
			root = p
		}
	}
	if !path.IsAbs(root) {
		return quotaSpace{}, err
	}
	return quotaSpace{root: path.Clean(root)}, nil
}

func (sp quotaSpace) args() map[string]string {
	args := map[string]string{"root": sp.root, "space_type": sp.spaceType, "owner": sp.owner}
	maps.DeleteFunc(args, func(_, v string) bool { return v == "" })
	return args
}

// invokeQuota runs a quota invocation on one instance of the mount.
func invokeQuota(ctx context.Context, eps []client.Endpoint, invocation string, sp quotaSpace, args map[string]string) (*adminpb.SpaceQuota, error) {
	res, err := invokeOne(ctx, eps, invocation, args)
	if err != nil {
		return nil, err
	}
	q := &adminpb.SpaceQuota{}
	if err := decode(res.ResultJSON, q); err != nil {
		return nil, err
	}
	if sp.id != nil {
		q.Space = &adminpb.SpaceRef{Ref: &adminpb.SpaceRef_Id{Id: sp.id}}
	}
	return q, nil
}
