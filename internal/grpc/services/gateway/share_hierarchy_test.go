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

package gateway

import (
	"context"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registry "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/permissions"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

const (
	shareHierarchyStorageID = "storage"
	shareHierarchySpaceID   = "space"
	userAOpaqueID           = "user-a"
	groupOpaqueID           = "group-a"

	parentPath     = "/Share hierarchy/Parent"
	childPath      = "/Share hierarchy/Parent/Child"
	grandchildPath = "/Share hierarchy/Parent/Child/Grandchild"
	siblingPath    = "/Share hierarchy/Sibling"
)

type shareHierarchyGatewayHarness struct {
	svc     *svc
	shares  *fakeCollaborationServer
	storage *fakeStorageProviderServer
}

func newShareHierarchyGatewayHarness(t *testing.T) *shareHierarchyGatewayHarness {
	t.Helper()

	storage := newFakeStorageProviderServer()
	storageEndpoint := t.Name() + "/storage"
	pool.RegisterStorageProviderServiceClient(storage, storageEndpoint)

	registryServer := &fakeStorageRegistryServer{storageEndpoint: storageEndpoint}
	registryEndpoint := t.Name() + "/registry"
	pool.RegisterStorageRegistryClient(registryServer, registryEndpoint)

	shares := newFakeCollaborationServer()
	shareEndpoint := t.Name() + "/shares"
	pool.RegisterUserShareProviderClient(shares, shareEndpoint)

	return &shareHierarchyGatewayHarness{
		svc: &svc{
			c: &config{
				UserShareProviderEndpoint: shareEndpoint,
				StorageRegistryEndpoint:   registryEndpoint,
				CommitShareToStorageGrant: true,
			},
		},
		shares:  shares,
		storage: storage,
	}
}

func TestShareHierarchyEndUserScenarios(t *testing.T) {
	t.Run("simple share works", func(t *testing.T) {
		h := newShareHierarchyGatewayHarness(t)

		h.createShare(t, parentPath, userAGrantee(), permissions.NewViewerRole().CS3ResourcePermissions(), false)

		h.assertCanRead(t, userAGrantee(), parentPath)
		h.assertCanRead(t, userAGrantee(), childPath)
		h.assertCannotWrite(t, userAGrantee(), parentPath)
	})

	t.Run("stronger child share is allowed", func(t *testing.T) {
		h := newShareHierarchyGatewayHarness(t)

		h.createShare(t, parentPath, userAGrantee(), permissions.NewViewerRole().CS3ResourcePermissions(), false)
		h.createShare(t, childPath, userAGrantee(), permissions.NewEditorRole().CS3ResourcePermissions(), false)

		h.assertCanRead(t, userAGrantee(), parentPath)
		h.assertCannotWrite(t, userAGrantee(), parentPath)
		h.assertCanWrite(t, userAGrantee(), childPath)
		h.assertSharePaths(t, userAGrantee(), parentPath, childPath)
	})

	t.Run("redundant child share is blocked", func(t *testing.T) {
		h := newShareHierarchyGatewayHarness(t)

		h.createShare(t, parentPath, userAGrantee(), permissions.NewViewerRole().CS3ResourcePermissions(), false)
		res := h.createShareResponse(t, childPath, userAGrantee(), permissions.NewViewerRole().CS3ResourcePermissions(), false)

		require.Equal(t, rpc.Code_CODE_ABORTED, res.Status.Code)
		assert.Contains(t, res.Status.Message, "already accessible")
		h.assertSharePaths(t, userAGrantee(), parentPath)
	})

	t.Run("weaker child share is blocked", func(t *testing.T) {
		h := newShareHierarchyGatewayHarness(t)

		h.createShare(t, parentPath, userAGrantee(), permissions.NewEditorRole().CS3ResourcePermissions(), false)
		res := h.createShareResponse(t, childPath, userAGrantee(), permissions.NewViewerRole().CS3ResourcePermissions(), false)

		require.Equal(t, rpc.Code_CODE_ABORTED, res.Status.Code)
		assert.Contains(t, res.Status.Message, "already accessible")
		h.assertCanWrite(t, userAGrantee(), childPath)
		h.assertSharePaths(t, userAGrantee(), parentPath)
	})

	t.Run("parent share warns about existing child share", func(t *testing.T) {
		h := newShareHierarchyGatewayHarness(t)

		h.createShare(t, childPath, userAGrantee(), permissions.NewViewerRole().CS3ResourcePermissions(), false)
		res := h.createShareResponse(t, parentPath, userAGrantee(), permissions.NewViewerRole().CS3ResourcePermissions(), false)

		require.Equal(t, rpc.Code_CODE_ABORTED, res.Status.Code)
		assert.Contains(t, res.Status.Message, "requires the deletion")
		h.assertSharePaths(t, userAGrantee(), childPath)
		h.assertCannotRead(t, userAGrantee(), parentPath)
	})

	t.Run("forced parent share removes redundant child share", func(t *testing.T) {
		h := newShareHierarchyGatewayHarness(t)

		h.createShare(t, childPath, userAGrantee(), permissions.NewViewerRole().CS3ResourcePermissions(), false)
		h.createShare(t, parentPath, userAGrantee(), permissions.NewViewerRole().CS3ResourcePermissions(), true)

		h.assertCanRead(t, userAGrantee(), childPath)
		h.assertCannotWrite(t, userAGrantee(), childPath)
		h.assertSharePaths(t, userAGrantee(), parentPath)
	})

	t.Run("forced parent share preserves stronger child share", func(t *testing.T) {
		h := newShareHierarchyGatewayHarness(t)

		h.createShare(t, childPath, userAGrantee(), permissions.NewEditorRole().CS3ResourcePermissions(), false)
		h.createShare(t, parentPath, userAGrantee(), permissions.NewViewerRole().CS3ResourcePermissions(), true)

		h.assertCanRead(t, userAGrantee(), parentPath)
		h.assertCannotWrite(t, userAGrantee(), parentPath)
		h.assertCanWrite(t, userAGrantee(), childPath)
		h.assertSharePaths(t, userAGrantee(), parentPath, childPath)
	})

	t.Run("sibling shares do not interfere", func(t *testing.T) {
		h := newShareHierarchyGatewayHarness(t)

		h.createShare(t, siblingPath, userAGrantee(), permissions.NewViewerRole().CS3ResourcePermissions(), false)
		h.createShare(t, parentPath, userAGrantee(), permissions.NewViewerRole().CS3ResourcePermissions(), false)

		h.assertCanRead(t, userAGrantee(), siblingPath)
		h.assertCanRead(t, userAGrantee(), parentPath)
		h.assertSharePaths(t, userAGrantee(), parentPath, siblingPath)
	})

	t.Run("updating a share keeps access correct", func(t *testing.T) {
		h := newShareHierarchyGatewayHarness(t)

		share := h.createShare(t, parentPath, userAGrantee(), permissions.NewViewerRole().CS3ResourcePermissions(), false)
		h.assertCannotWrite(t, userAGrantee(), parentPath)

		h.updateShare(t, share.Id, permissions.NewEditorRole().CS3ResourcePermissions(), false)
		h.assertCanWrite(t, userAGrantee(), parentPath)

		h.updateShare(t, share.Id, permissions.NewViewerRole().CS3ResourcePermissions(), false)
		h.assertCanRead(t, userAGrantee(), parentPath)
		h.assertCannotWrite(t, userAGrantee(), parentPath)
	})

	t.Run("group shares follow the same hierarchy rules", func(t *testing.T) {
		h := newShareHierarchyGatewayHarness(t)

		h.createShare(t, parentPath, groupGrantee(), permissions.NewViewerRole().CS3ResourcePermissions(), false)
		res := h.createShareResponse(t, childPath, groupGrantee(), permissions.NewViewerRole().CS3ResourcePermissions(), false)

		require.Equal(t, rpc.Code_CODE_ABORTED, res.Status.Code)
		assert.Contains(t, res.Status.Message, "already accessible")
		h.assertCanRead(t, groupGrantee(), parentPath)
		h.assertCanRead(t, groupGrantee(), childPath)
		h.assertCannotWrite(t, groupGrantee(), childPath)
		h.assertSharePaths(t, groupGrantee(), parentPath)
	})
}

func (h *shareHierarchyGatewayHarness) createShare(t *testing.T, p string, grantee *provider.Grantee, perms *provider.ResourcePermissions, force bool) *collaboration.Share {
	t.Helper()
	res := h.createShareResponse(t, p, grantee, perms, force)
	require.Equal(t, rpc.Code_CODE_OK, res.Status.Code, res.Status.Message)
	require.NotNil(t, res.Share)
	return res.Share
}

func (h *shareHierarchyGatewayHarness) createShareResponse(t *testing.T, p string, grantee *provider.Grantee, perms *provider.ResourcePermissions, force bool) *collaboration.CreateShareResponse {
	t.Helper()
	res, err := h.svc.CreateShare(context.Background(), &collaboration.CreateShareRequest{
		ResourceInfo: h.storage.resourceInfo(p),
		Grant: &collaboration.ShareGrant{
			Grantee: grantee,
			Permissions: &collaboration.SharePermissions{
				Permissions: perms,
			},
		},
		Opaque: forceOpaque(force),
	})
	require.NoError(t, err)
	require.NotNil(t, res.Status)
	return res
}

func (h *shareHierarchyGatewayHarness) updateShare(t *testing.T, id *collaboration.ShareId, perms *provider.ResourcePermissions, force bool) {
	t.Helper()
	res, err := h.svc.UpdateShare(context.Background(), &collaboration.UpdateShareRequest{
		Ref: &collaboration.ShareReference{
			Spec: &collaboration.ShareReference_Id{Id: id},
		},
		Field: &collaboration.UpdateShareRequest_UpdateField{
			Field: &collaboration.UpdateShareRequest_UpdateField_Permissions{
				Permissions: &collaboration.SharePermissions{Permissions: perms},
			},
		},
		Opaque: forceOpaque(force),
	})
	require.NoError(t, err)
	require.Equal(t, rpc.Code_CODE_OK, res.Status.Code, res.Status.Message)
}

func (h *shareHierarchyGatewayHarness) assertSharePaths(t *testing.T, grantee *provider.Grantee, want ...string) {
	t.Helper()
	assert.Equal(t, sortedStrings(want), h.shares.pathsFor(grantee))
}

func (h *shareHierarchyGatewayHarness) assertCanRead(t *testing.T, grantee *provider.Grantee, p string) {
	t.Helper()
	assert.True(t, h.storage.effectivePermissions(grantee, p).Stat, "expected read access for %s at %s", granteeKey(grantee), p)
}

func (h *shareHierarchyGatewayHarness) assertCannotRead(t *testing.T, grantee *provider.Grantee, p string) {
	t.Helper()
	assert.False(t, h.storage.effectivePermissions(grantee, p).Stat, "expected no read access for %s at %s", granteeKey(grantee), p)
}

func (h *shareHierarchyGatewayHarness) assertCanWrite(t *testing.T, grantee *provider.Grantee, p string) {
	t.Helper()
	assert.True(t, h.storage.effectivePermissions(grantee, p).InitiateFileUpload, "expected write access for %s at %s", granteeKey(grantee), p)
}

func (h *shareHierarchyGatewayHarness) assertCannotWrite(t *testing.T, grantee *provider.Grantee, p string) {
	t.Helper()
	assert.False(t, h.storage.effectivePermissions(grantee, p).InitiateFileUpload, "expected no write access for %s at %s", granteeKey(grantee), p)
}

type fakeCollaborationServer struct {
	collaboration.CollaborationAPIClient

	mu     sync.Mutex
	nextID int
	shares map[string]*collaboration.Share
}

func newFakeCollaborationServer() *fakeCollaborationServer {
	return &fakeCollaborationServer{
		nextID: 1,
		shares: map[string]*collaboration.Share{},
	}
}

func (s *fakeCollaborationServer) CreateShare(_ context.Context, req *collaboration.CreateShareRequest, _ ...grpc.CallOption) (*collaboration.CreateShareResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := strconv.Itoa(s.nextID)
	s.nextID++
	share := proto.Clone(&collaboration.Share{
		Id:          &collaboration.ShareId{OpaqueId: id},
		ResourceId:  req.ResourceInfo.Id,
		Grantee:     req.Grant.Grantee,
		Permissions: &collaboration.SharePermissions{Permissions: req.Grant.Permissions.Permissions},
	}).(*collaboration.Share)
	s.shares[id] = share
	return &collaboration.CreateShareResponse{Status: okStatus(), Share: proto.Clone(share).(*collaboration.Share)}, nil
}

func (s *fakeCollaborationServer) RemoveShare(_ context.Context, req *collaboration.RemoveShareRequest, _ ...grpc.CallOption) (*collaboration.RemoveShareResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := req.Ref.GetId().OpaqueId
	if _, ok := s.shares[id]; !ok {
		return &collaboration.RemoveShareResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}, nil
	}
	delete(s.shares, id)
	return &collaboration.RemoveShareResponse{Status: okStatus()}, nil
}

func (s *fakeCollaborationServer) GetShare(_ context.Context, req *collaboration.GetShareRequest, _ ...grpc.CallOption) (*collaboration.GetShareResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := req.Ref.GetId().OpaqueId
	share, ok := s.shares[id]
	if !ok {
		return nil, errtypes.NotFound(id)
	}
	return &collaboration.GetShareResponse{Status: okStatus(), Share: proto.Clone(share).(*collaboration.Share)}, nil
}

func (s *fakeCollaborationServer) ListShares(_ context.Context, req *collaboration.ListSharesRequest, _ ...grpc.CallOption) (*collaboration.ListSharesResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	shares := make([]*collaboration.Share, 0, len(s.shares))
	for _, share := range s.shares {
		if shareMatchesFilters(share, req.Filters) {
			shares = append(shares, proto.Clone(share).(*collaboration.Share))
		}
	}
	return &collaboration.ListSharesResponse{Status: okStatus(), Shares: shares}, nil
}

func (s *fakeCollaborationServer) UpdateShare(_ context.Context, req *collaboration.UpdateShareRequest, _ ...grpc.CallOption) (*collaboration.UpdateShareResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := req.Ref.GetId().OpaqueId
	share, ok := s.shares[id]
	if !ok {
		return &collaboration.UpdateShareResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}, nil
	}
	if perms := req.Field.GetPermissions(); perms != nil {
		share.Permissions = &collaboration.SharePermissions{Permissions: perms.Permissions}
	}
	return &collaboration.UpdateShareResponse{Status: okStatus(), Share: proto.Clone(share).(*collaboration.Share)}, nil
}

func (s *fakeCollaborationServer) pathsFor(grantee *provider.Grantee) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	var paths []string
	for _, share := range s.shares {
		if sameGrantee(share.Grantee, grantee) {
			paths = append(paths, pathForOpaqueID(share.ResourceId.OpaqueId))
		}
	}
	return sortedStrings(paths)
}

type fakeStorageRegistryServer struct {
	registry.RegistryAPIClient
	storageEndpoint string
}

func (s *fakeStorageRegistryServer) GetStorageProviders(context.Context, *registry.GetStorageProvidersRequest, ...grpc.CallOption) (*registry.GetStorageProvidersResponse, error) {
	return &registry.GetStorageProvidersResponse{
		Status: okStatus(),
		Providers: []*registry.ProviderInfo{
			{
				Address: s.storageEndpoint,
			},
		},
	}, nil
}

type fakeStorageProviderServer struct {
	provider.ProviderAPIClient

	mu     sync.Mutex
	grants map[string]map[string]*provider.ResourcePermissions
}

func newFakeStorageProviderServer() *fakeStorageProviderServer {
	return &fakeStorageProviderServer{
		grants: map[string]map[string]*provider.ResourcePermissions{},
	}
}

func (s *fakeStorageProviderServer) AddGrant(_ context.Context, req *provider.AddGrantRequest, _ ...grpc.CallOption) (*provider.AddGrantResponse, error) {
	s.setGrant(req.Ref.GetResourceId().OpaqueId, req.Grant.Grantee, req.Grant.Permissions)
	return &provider.AddGrantResponse{Status: okStatus()}, nil
}

func (s *fakeStorageProviderServer) UpdateGrant(_ context.Context, req *provider.UpdateGrantRequest, _ ...grpc.CallOption) (*provider.UpdateGrantResponse, error) {
	s.setGrant(req.Ref.GetResourceId().OpaqueId, req.Grant.Grantee, req.Grant.Permissions)
	return &provider.UpdateGrantResponse{Status: okStatus()}, nil
}

func (s *fakeStorageProviderServer) RemoveGrant(_ context.Context, req *provider.RemoveGrantRequest, _ ...grpc.CallOption) (*provider.RemoveGrantResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.grants[req.Ref.GetResourceId().OpaqueId], granteeKey(req.Grant.Grantee))
	return &provider.RemoveGrantResponse{Status: okStatus()}, nil
}

func (s *fakeStorageProviderServer) DenyGrant(_ context.Context, req *provider.DenyGrantRequest, _ ...grpc.CallOption) (*provider.DenyGrantResponse, error) {
	s.setGrant(req.Ref.GetResourceId().OpaqueId, req.Grantee, &provider.ResourcePermissions{})
	return &provider.DenyGrantResponse{Status: okStatus()}, nil
}

func (s *fakeStorageProviderServer) Stat(_ context.Context, req *provider.StatRequest, _ ...grpc.CallOption) (*provider.StatResponse, error) {
	p := pathForOpaqueID(req.Ref.GetResourceId().OpaqueId)
	return &provider.StatResponse{Status: okStatus(), Info: s.resourceInfo(p)}, nil
}

func (s *fakeStorageProviderServer) GetPath(_ context.Context, req *provider.GetPathRequest, _ ...grpc.CallOption) (*provider.GetPathResponse, error) {
	return &provider.GetPathResponse{
		Status: okStatus(),
		Path:   pathForOpaqueID(req.ResourceId.OpaqueId),
	}, nil
}

func (s *fakeStorageProviderServer) setGrant(opaqueID string, grantee *provider.Grantee, perms *provider.ResourcePermissions) {
	s.mu.Lock()
	defer s.mu.Unlock()

	targetPath := pathForOpaqueID(opaqueID)
	for currentOpaqueID := range storageTree() {
		if currentOpaqueID != opaqueID && !isSameOrDescendant(targetPath, pathForOpaqueID(currentOpaqueID)) {
			continue
		}
		if s.grants[currentOpaqueID] == nil {
			s.grants[currentOpaqueID] = map[string]*provider.ResourcePermissions{}
		}
		s.grants[currentOpaqueID][granteeKey(grantee)] = proto.Clone(perms).(*provider.ResourcePermissions)
	}
}

func (s *fakeStorageProviderServer) effectivePermissions(grantee *provider.Grantee, p string) *provider.ResourcePermissions {
	s.mu.Lock()
	defer s.mu.Unlock()

	for current := p; current != "." && current != "/"; current = path.Dir(current) {
		perms, ok := s.grants[opaqueIDForPath(current)][granteeKey(grantee)]
		if !ok {
			continue
		}
		return proto.Clone(perms).(*provider.ResourcePermissions)
	}
	return &provider.ResourcePermissions{}
}

func (s *fakeStorageProviderServer) resourceInfo(p string) *provider.ResourceInfo {
	return &provider.ResourceInfo{
		Id: &provider.ResourceId{
			StorageId: shareHierarchyStorageID,
			SpaceId:   shareHierarchySpaceID,
			OpaqueId:  opaqueIDForPath(p),
		},
		Path: p,
		Type: provider.ResourceType_RESOURCE_TYPE_CONTAINER,
	}
}

func shareMatchesFilters(share *collaboration.Share, filters []*collaboration.Filter) bool {
	for _, filter := range filters {
		switch filter.Type {
		case collaboration.Filter_TYPE_SPACE_ID:
			if share.ResourceId.SpaceId != filter.GetSpaceId() {
				return false
			}
		case collaboration.Filter_TYPE_GRANTEE:
			if !sameGrantee(share.Grantee, filter.GetGrantee()) {
				return false
			}
		}
	}
	return true
}

func sameGrantee(a, b *provider.Grantee) bool {
	return granteeKey(a) == granteeKey(b)
}

func granteeKey(g *provider.Grantee) string {
	if g.GetUserId() != nil {
		return "user:" + g.GetUserId().OpaqueId
	}
	if g.GetGroupId() != nil {
		return "group:" + g.GetGroupId().OpaqueId
	}
	return "invalid"
}

func userAGrantee() *provider.Grantee {
	return &provider.Grantee{
		Type: provider.GranteeType_GRANTEE_TYPE_USER,
		Id: &provider.Grantee_UserId{
			UserId: &userpb.UserId{OpaqueId: userAOpaqueID},
		},
	}
}

func groupGrantee() *provider.Grantee {
	return &provider.Grantee{
		Type: provider.GranteeType_GRANTEE_TYPE_GROUP,
		Id: &provider.Grantee_GroupId{
			GroupId: &grouppb.GroupId{OpaqueId: groupOpaqueID},
		},
	}
}

func forceOpaque(force bool) *types.Opaque {
	if !force {
		return nil
	}
	return &types.Opaque{
		Map: map[string]*types.OpaqueEntry{
			"force": {Decoder: "plain", Value: []byte("true")},
		},
	}
}

func okStatus() *rpc.Status {
	return &rpc.Status{Code: rpc.Code_CODE_OK}
}

func opaqueIDForPath(p string) string {
	cleaned := strings.Trim(path.Clean(p), "/")
	if cleaned == "" {
		return "root"
	}
	return strings.ReplaceAll(cleaned, "/", "_")
}

func pathForOpaqueID(opaqueID string) string {
	if p, ok := storageTree()[opaqueID]; ok {
		return p
	}
	return "/" + strings.ReplaceAll(opaqueID, "_", "/")
}

func storageTree() map[string]string {
	return map[string]string{
		opaqueIDForPath(parentPath):     parentPath,
		opaqueIDForPath(childPath):      childPath,
		opaqueIDForPath(grandchildPath): grandchildPath,
		opaqueIDForPath(siblingPath):    siblingPath,
	}
}

func isSameOrDescendant(parent, child string) bool {
	parent = path.Clean(parent)
	child = path.Clean(child)
	return child == parent || strings.HasPrefix(child, parent+"/")
}

func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
