// Copyright 2018-2021 CERN
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
	"encoding/json"
	"strings"
	"time"

	"github.com/ReneKroon/ttlcache/v2"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registry "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	sdk "github.com/cs3org/reva/pkg/sdk/common"
	"github.com/cs3org/reva/pkg/utils"
	"google.golang.org/grpc"
)

// generates a user specific key pointing to ref
func userKey(user *userpb.User, ref *provider.Reference) string {
	key := "uid" + user.Id.OpaqueId
	if ref == nil || ref.ResourceId == nil || ref.ResourceId.StorageId == "" {
		return key
	}

	return key + "!sid:" + ref.ResourceId.StorageId + "!oid:" + ref.ResourceId.OpaqueId + "!path:" + ref.Path
}

// Caches holds all caches used by the gateway
type Caches struct {
	statCache     *ttlcache.Cache
	homeCache     *ttlcache.Cache
	providerCache *ttlcache.Cache
}

// NewCaches initializes a Caches struct for easy use
func NewCaches(conf *config) *Caches {
	return &Caches{
		homeCache:     initCache(conf.CreateHomeCacheTTL),
		statCache:     initCache(conf.StatCacheTTL),
		providerCache: initCache(conf.ProviderCacheTTL),
	}
}

// StorageProviderClient returns a (cached) client pointing to the storageprovider
func (c *Caches) StorageProviderClient(p provider.ProviderAPIClient) provider.ProviderAPIClient {
	return &cachedAPIClient{
		c:         p,
		statCache: c.statCache,
		homeCache: c.homeCache,
	}
}

// StorageRegistryClient returns a (cached) client pointing to the storageregistry
func (c *Caches) StorageRegistryClient(p registry.RegistryAPIClient) registry.RegistryAPIClient {
	//return &cachedRegistryClient{
	//c:             p,
	//providerCache: c.providerCache,
	//}
	return p
}

// RemoveStat removes a reference from the stat cache
func (c *Caches) RemoveStat(user *userpb.User, res *provider.ResourceId) {
	uid := "uid:" + user.Id.OpaqueId
	sid := ""
	oid := ""
	if res != nil {
		sid = "sid:" + res.StorageId
		oid = "oid:" + res.OpaqueId
	}

	for _, key := range c.statCache.GetKeys() {
		if strings.Contains(key, uid) {
			_ = c.statCache.Remove(key)
			continue
		}

		if sid != "" && strings.Contains(key, sid) {
			_ = c.statCache.Remove(key)
			continue
		}

		if oid != "" && strings.Contains(key, oid) {
			_ = c.statCache.Remove(key)
			continue
		}
	}
}

func initCache(ttlSeconds int) *ttlcache.Cache {
	cache := ttlcache.NewCache()
	_ = cache.SetTTL(time.Duration(ttlSeconds) * time.Second)
	cache.SkipTTLExtensionOnHit(true)
	return cache
}

func pullFromCache(cache *ttlcache.Cache, key string, dest interface{}) error {
	r, err := cache.Get(key)
	if err != nil {
		return err
	}

	return json.Unmarshal(r.([]byte), dest)
}

func pushToCache(cache *ttlcache.Cache, key string, src interface{}) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return cache.Set(key, b)
}

/*
   Cached Registry
*/

type cachedRegistryClient struct {
	c             registry.RegistryAPIClient
	providerCache *ttlcache.Cache
}

func (c *cachedRegistryClient) ListStorageProviders(ctx context.Context, in *registry.ListStorageProvidersRequest, opts ...grpc.CallOption) (*registry.ListStorageProvidersResponse, error) {
	key := sdk.DecodeOpaqueMap(in.Opaque)["storage_id"]
	if key != "" {
		s := &registry.ListStorageProvidersResponse{}
		if err := pullFromCache(c.providerCache, key, s); err == nil {
			return s, nil
		}
	}

	resp, err := c.c.ListStorageProviders(ctx, in, opts...)
	switch {
	case err != nil:
		return nil, err
	case resp.Status.Code != rpc.Code_CODE_OK && resp.Status.Code != rpc.Code_CODE_NOT_FOUND:
		return resp, nil
	case key == "":
		return resp, nil
	default:
		return resp, pushToCache(c.providerCache, key, resp)
	}
}

// not cached

func (c *cachedRegistryClient) GetStorageProviders(ctx context.Context, in *registry.GetStorageProvidersRequest, opts ...grpc.CallOption) (*registry.GetStorageProvidersResponse, error) {
	return c.c.GetStorageProviders(ctx, in, opts...)
}

func (c *cachedRegistryClient) GetHome(ctx context.Context, in *registry.GetHomeRequest, opts ...grpc.CallOption) (*registry.GetHomeResponse, error) {
	return c.c.GetHome(ctx, in, opts...)
}

/*
   Cached Storage Provider
*/

type cachedAPIClient struct {
	c         provider.ProviderAPIClient
	statCache *ttlcache.Cache
	homeCache *ttlcache.Cache
}

// Stat looks in cache first before forwarding to storage provider
func (c *cachedAPIClient) Stat(ctx context.Context, in *provider.StatRequest, opts ...grpc.CallOption) (*provider.StatResponse, error) {
	key := userKey(ctxpkg.ContextMustGetUser(ctx), in.Ref)
	if key != "" {
		s := &provider.StatResponse{}
		if err := pullFromCache(c.statCache, key, s); err == nil {
			return s, nil
		}
	}
	resp, err := c.c.Stat(ctx, in, opts...)
	switch {
	case err != nil:
		return nil, err
	case resp.Status.Code != rpc.Code_CODE_OK && resp.Status.Code != rpc.Code_CODE_NOT_FOUND:
		return resp, nil
	case key == "":
		return resp, nil
	case strings.Contains(key, "sid:"+utils.ShareStorageProviderID):
		// We cannot cache shares at the moment:
		// we do not know when to invalidate them
		// FIXME: find a way to cache/invalidate them too
		return resp, nil
	default:
		return resp, pushToCache(c.statCache, key, resp)
	}
}

// CreateHome caches calls to CreateHome locally - anyways they only need to be called once per user
func (c *cachedAPIClient) CreateHome(ctx context.Context, in *provider.CreateHomeRequest, opts ...grpc.CallOption) (*provider.CreateHomeResponse, error) {
	key := ctxpkg.ContextMustGetUser(ctx).Id.OpaqueId
	if key != "" {
		s := &provider.CreateHomeResponse{}
		if err := pullFromCache(c.homeCache, key, s); err == nil {
			return s, nil
		}

	}
	resp, err := c.c.CreateHome(ctx, in, opts...)
	switch {
	case err != nil:
		return nil, err
	case resp.Status.Code != rpc.Code_CODE_OK && resp.Status.Code != rpc.Code_CODE_ALREADY_EXISTS:
		return resp, nil
	case key == "":
		return resp, nil
	default:
		return resp, pushToCache(c.homeCache, key, resp)
	}
}

// methods below here are not cached, they just call the client directly

func (c *cachedAPIClient) AddGrant(ctx context.Context, in *provider.AddGrantRequest, opts ...grpc.CallOption) (*provider.AddGrantResponse, error) {
	return c.c.AddGrant(ctx, in, opts...)
}
func (c *cachedAPIClient) CreateContainer(ctx context.Context, in *provider.CreateContainerRequest, opts ...grpc.CallOption) (*provider.CreateContainerResponse, error) {
	return c.c.CreateContainer(ctx, in, opts...)
}
func (c *cachedAPIClient) Delete(ctx context.Context, in *provider.DeleteRequest, opts ...grpc.CallOption) (*provider.DeleteResponse, error) {
	return c.c.Delete(ctx, in, opts...)
}
func (c *cachedAPIClient) DenyGrant(ctx context.Context, in *provider.DenyGrantRequest, opts ...grpc.CallOption) (*provider.DenyGrantResponse, error) {
	return c.c.DenyGrant(ctx, in, opts...)
}
func (c *cachedAPIClient) GetPath(ctx context.Context, in *provider.GetPathRequest, opts ...grpc.CallOption) (*provider.GetPathResponse, error) {
	return c.c.GetPath(ctx, in, opts...)
}
func (c *cachedAPIClient) GetQuota(ctx context.Context, in *provider.GetQuotaRequest, opts ...grpc.CallOption) (*provider.GetQuotaResponse, error) {
	return c.c.GetQuota(ctx, in, opts...)
}
func (c *cachedAPIClient) InitiateFileDownload(ctx context.Context, in *provider.InitiateFileDownloadRequest, opts ...grpc.CallOption) (*provider.InitiateFileDownloadResponse, error) {
	return c.c.InitiateFileDownload(ctx, in, opts...)
}
func (c *cachedAPIClient) InitiateFileUpload(ctx context.Context, in *provider.InitiateFileUploadRequest, opts ...grpc.CallOption) (*provider.InitiateFileUploadResponse, error) {
	return c.c.InitiateFileUpload(ctx, in, opts...)
}
func (c *cachedAPIClient) ListGrants(ctx context.Context, in *provider.ListGrantsRequest, opts ...grpc.CallOption) (*provider.ListGrantsResponse, error) {
	return c.c.ListGrants(ctx, in, opts...)
}
func (c *cachedAPIClient) ListContainerStream(ctx context.Context, in *provider.ListContainerStreamRequest, opts ...grpc.CallOption) (provider.ProviderAPI_ListContainerStreamClient, error) {
	return c.c.ListContainerStream(ctx, in, opts...)
}
func (c *cachedAPIClient) ListContainer(ctx context.Context, in *provider.ListContainerRequest, opts ...grpc.CallOption) (*provider.ListContainerResponse, error) {
	return c.c.ListContainer(ctx, in, opts...)
}
func (c *cachedAPIClient) ListFileVersions(ctx context.Context, in *provider.ListFileVersionsRequest, opts ...grpc.CallOption) (*provider.ListFileVersionsResponse, error) {
	return c.c.ListFileVersions(ctx, in, opts...)
}
func (c *cachedAPIClient) ListRecycleStream(ctx context.Context, in *provider.ListRecycleStreamRequest, opts ...grpc.CallOption) (provider.ProviderAPI_ListRecycleStreamClient, error) {
	return c.c.ListRecycleStream(ctx, in, opts...)
}
func (c *cachedAPIClient) ListRecycle(ctx context.Context, in *provider.ListRecycleRequest, opts ...grpc.CallOption) (*provider.ListRecycleResponse, error) {
	return c.c.ListRecycle(ctx, in, opts...)
}
func (c *cachedAPIClient) Move(ctx context.Context, in *provider.MoveRequest, opts ...grpc.CallOption) (*provider.MoveResponse, error) {
	return c.c.Move(ctx, in, opts...)
}
func (c *cachedAPIClient) RemoveGrant(ctx context.Context, in *provider.RemoveGrantRequest, opts ...grpc.CallOption) (*provider.RemoveGrantResponse, error) {
	return c.c.RemoveGrant(ctx, in, opts...)
}
func (c *cachedAPIClient) PurgeRecycle(ctx context.Context, in *provider.PurgeRecycleRequest, opts ...grpc.CallOption) (*provider.PurgeRecycleResponse, error) {
	return c.c.PurgeRecycle(ctx, in, opts...)
}
func (c *cachedAPIClient) RestoreFileVersion(ctx context.Context, in *provider.RestoreFileVersionRequest, opts ...grpc.CallOption) (*provider.RestoreFileVersionResponse, error) {
	return c.c.RestoreFileVersion(ctx, in, opts...)
}
func (c *cachedAPIClient) RestoreRecycleItem(ctx context.Context, in *provider.RestoreRecycleItemRequest, opts ...grpc.CallOption) (*provider.RestoreRecycleItemResponse, error) {
	return c.c.RestoreRecycleItem(ctx, in, opts...)
}
func (c *cachedAPIClient) UpdateGrant(ctx context.Context, in *provider.UpdateGrantRequest, opts ...grpc.CallOption) (*provider.UpdateGrantResponse, error) {
	return c.c.UpdateGrant(ctx, in, opts...)
}
func (c *cachedAPIClient) CreateSymlink(ctx context.Context, in *provider.CreateSymlinkRequest, opts ...grpc.CallOption) (*provider.CreateSymlinkResponse, error) {
	return c.c.CreateSymlink(ctx, in, opts...)
}
func (c *cachedAPIClient) CreateReference(ctx context.Context, in *provider.CreateReferenceRequest, opts ...grpc.CallOption) (*provider.CreateReferenceResponse, error) {
	return c.c.CreateReference(ctx, in, opts...)
}
func (c *cachedAPIClient) SetArbitraryMetadata(ctx context.Context, in *provider.SetArbitraryMetadataRequest, opts ...grpc.CallOption) (*provider.SetArbitraryMetadataResponse, error) {
	return c.c.SetArbitraryMetadata(ctx, in, opts...)
}
func (c *cachedAPIClient) UnsetArbitraryMetadata(ctx context.Context, in *provider.UnsetArbitraryMetadataRequest, opts ...grpc.CallOption) (*provider.UnsetArbitraryMetadataResponse, error) {
	return c.c.UnsetArbitraryMetadata(ctx, in, opts...)
}
func (c *cachedAPIClient) GetHome(ctx context.Context, in *provider.GetHomeRequest, opts ...grpc.CallOption) (*provider.GetHomeResponse, error) {
	return c.c.GetHome(ctx, in, opts...)
}
func (c *cachedAPIClient) CreateStorageSpace(ctx context.Context, in *provider.CreateStorageSpaceRequest, opts ...grpc.CallOption) (*provider.CreateStorageSpaceResponse, error) {
	return c.c.CreateStorageSpace(ctx, in, opts...)
}
func (c *cachedAPIClient) ListStorageSpaces(ctx context.Context, in *provider.ListStorageSpacesRequest, opts ...grpc.CallOption) (*provider.ListStorageSpacesResponse, error) {
	return c.c.ListStorageSpaces(ctx, in, opts...)
}
func (c *cachedAPIClient) UpdateStorageSpace(ctx context.Context, in *provider.UpdateStorageSpaceRequest, opts ...grpc.CallOption) (*provider.UpdateStorageSpaceResponse, error) {
	return c.c.UpdateStorageSpace(ctx, in, opts...)
}
func (c *cachedAPIClient) DeleteStorageSpace(ctx context.Context, in *provider.DeleteStorageSpaceRequest, opts ...grpc.CallOption) (*provider.DeleteStorageSpaceResponse, error) {
	return c.c.DeleteStorageSpace(ctx, in, opts...)
}
