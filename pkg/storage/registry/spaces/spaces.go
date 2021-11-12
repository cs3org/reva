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

package spaces

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registrypb "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/storage"
	pkgregistry "github.com/cs3org/reva/pkg/storage/registry/registry"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/mitchellh/mapstructure"
)

func init() {
	pkgregistry.Register("spaces", New)
}

type rule struct {
	Mapping           string            `mapstructure:"mapping"`
	Address           string            `mapstructure:"address"`
	Aliases           map[string]string `mapstructure:"aliases"`
	AllowedUserAgents []string          `mapstructure:"allowed_user_agents"`
}

type config struct {
	Rules        map[string]rule `mapstructure:"rules"`
	HomeProvider string          `mapstructure:"home_provider"`
}

func (c *config) init() {
	if c.HomeProvider == "" {
		c.HomeProvider = "/"
	}

	if len(c.Rules) == 0 {
		c.Rules = map[string]rule{
			"/": {
				Address: sharedconf.GetGatewaySVC(""),
			},
			"00000000-0000-0000-0000-000000000000": {
				Address: sharedconf.GetGatewaySVC(""),
			},
		}
	}
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

// New returns an implementation of the storage.Registry interface that
// redirects requests to corresponding storage drivers.
func New(m map[string]interface{}) (storage.Registry, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	c.init()
	return &registry{
		c:         c,
		resources: make(map[string][]*registrypb.ProviderInfo),
		//aliases:           make(map[string]map[string]*spaceAndProvider),
		resourceNameCache: make(map[string]string),
	}, nil
}

type spaceAndProvider struct {
	*provider.StorageSpace
	providers []*registrypb.ProviderInfo
}

type registry struct {
	c *config
	// a map of resources to providers
	resources map[string][]*registrypb.ProviderInfo
	// a map of paths/aliases to spaces and providers
	//aliases           map[string]map[string]*spaceAndProvider
	resourceNameCache map[string]string
}

// ListProviders lists all storage spaces, which is *very* different from the static provider, which lists provider ids
func (r *registry) ListProviders(ctx context.Context) ([]*registrypb.ProviderInfo, error) {
	// after init we have a list of storage provider addresses
	// 1. lazily fetch all storage spaces the current user can access by directly calling the provider
	providers := []*registrypb.ProviderInfo{}
	for _, rule := range r.c.Rules {
		c, err := pool.GetStorageProviderServiceClient(rule.Address)
		if err != nil {
			appctx.GetLogger(ctx).Warn().Err(err).Str("maping", rule.Mapping).Str("addr", rule.Address).Msg("GetStorageProviderServiceClient failed, continuing")
			continue
		}
		// TODO add filter to only query spaces the current user has access to? or leave permissions to the gateway?
		lSSRes, err := c.ListStorageSpaces(ctx, &provider.ListStorageSpacesRequest{ /*Filters: []*provider.ListStorageSpacesRequest_Filter{
				&provider.ListStorageSpacesRequest_Filter{
					Type: provider.ListStorageSpacesRequest_Filter_TYPE_ACCESS, ?
				},
			}*/})
		if err != nil {
			appctx.GetLogger(ctx).Warn().Err(err).Msg("ListStorageSpaces failed, continuing")
			continue
		}
		if lSSRes.Status.Code != rpc.Code_CODE_OK {
			appctx.GetLogger(ctx).Debug().Interface("status", lSSRes.Status).Msg("ListStorageSpaces was not OK, continuing")
			continue
		}
		for _, space := range lSSRes.StorageSpaces {
			pi := &registrypb.ProviderInfo{
				ProviderId:   space.Root.StorageId + "!" + space.Root.OpaqueId,
				ProviderPath: filepath.Join("/", space.SpaceType, space.Name), // TODO do we need to guarantee these are unique?
				Address:      rule.Address,
			}
			providers = append(providers, pi)
			r.resources[space.Root.StorageId+"!"+space.Root.OpaqueId] = []*registrypb.ProviderInfo{pi}
		}
	}
	return providers, nil
}

// returns the the root path of the first provider in the list.
func (r *registry) GetHome(ctx context.Context) (*registrypb.ProviderInfo, error) {
	if rule, ok := r.c.Rules[r.c.HomeProvider]; ok {
		return &registrypb.ProviderInfo{
			ProviderPath: r.c.HomeProvider,
			Address:      rule.Address,
		}, nil
	}
	return nil, errors.New("static: home not found")
}

// FindProviders will return all providers
// Given providers mounted at /home, /personal, /public, /shares, /foo and /foo/sub
// When a stat for / arrives
// Then the gateway needs all providers below /
// -> all providers
//
// When a stat for /home arrives
// Then the gateway needs all providers below /home
// -> only the /home provider
//
// When a stat for /foo arrives
// Then the gateway needs all providers below /foo
// -> the /foo and /foo/sub providers
//
// Given providers mounted at /foo, /foo/sub and /foo/sub/bar
// When a MKCOL for /foo/bif arrives
// Then the ocdav will make a stat for /foo/bif
// Then the gateway only needs the provider /foo
// -> only the /foo provider

// When a MKCOL for /foo/sub/mob arrives
// Then the ocdav will make a stat for /foo/sub/mob
// Then the gateway needs all providers below /foo/sub
// -> only the /foo/sub provider
//
//           requested path   provider path
// above   = /foo           <=> /foo/bar        -> stat(spaceid, .)    -> add metadata for /foo/bar
// above   = /foo           <=> /foo/bar/bif    -> stat(spaceid, .)    -> add metadata for /foo/bar
// matches = /foo/bar       <=> /foo/bar        -> list(spaceid, .)
// below   = /foo/bar/bif   <=> /foo/bar        -> list(spaceid, ./bif)
func (r *registry) FindProviders(ctx context.Context, ref *provider.Reference) ([]*registrypb.ProviderInfo, error) {
	switch {
	case ref.ResourceId != nil && ref.ResourceId.StorageId != "":
		return r.findProvidersForResource(ctx, ref.ResourceId)
	case utils.IsAbsolutePathReference(ref):
		return r.findProvidersForAbsolutePathReference(ctx, ref)
	default:
		return nil, errtypes.NotSupported("unsupported reference type")
	}
}
func (r *registry) findProvidersForResource(ctx context.Context, res *provider.ResourceId) ([]*registrypb.ProviderInfo, error) {
	// check if the resource is known
	if providers, ok := r.resources[res.StorageId+"!"+res.OpaqueId]; ok {
		// best case, just return cached provider
		return providers, nil
	}

	for path, rule := range r.c.Rules {
		p := &registrypb.ProviderInfo{
			Address:      rule.Address,
			ProviderPath: path,
		}
		resource, err := r.findResourceOnProvider(ctx, p, res)
		if err == nil && resource.Id != nil {
			p.ProviderId = resource.Id.StorageId + "!" + resource.Id.OpaqueId
			/*
				name := space.Name
				if name == "" {
					name = space.Id.OpaqueId
				}
				p.ProviderPath = filepath.Join("/", space.SpaceType, name) // TODO deduplicate name
			*/
			// cache result, TODO only for 30sec?
			r.resources[p.ProviderId] = []*registrypb.ProviderInfo{p}
			// TODO continue iterating to collect all providers that have access
			return []*registrypb.ProviderInfo{p}, nil
		}
	}
	return []*registrypb.ProviderInfo{}, nil
}

func (r *registry) findProvidersForAbsolutePathReference(ctx context.Context, ref *provider.Reference) ([]*registrypb.ProviderInfo, error) {
	currentUser := ctxpkg.ContextMustGetUser(ctx)
	// check if the alias is known for this user
	//spaceType, rest := router.ShiftPath(ref.Path)
	//spaceName, _ := router.ShiftPath(rest)
	//alias := filepath.Join("/", spaceType, spaceName)
	aliases := map[string]*spaceAndProvider{}
	/*
		if _, ok := r.aliases[currentUser.Id.OpaqueId]; !ok {
			r.aliases[currentUser.Id.OpaqueId] = make(map[string]*spaceAndProvider)
		}
			if spaceAndAddr, ok := r.aliases[currentUser.Id.OpaqueId][alias]; ok {
				// best case, just return cached provider
				return spaceAndAddr.providers, nil
			}
	*/

	// TODO  instead of replacing home with personal to reduce the amount of storage spaces returned by a storage provider
	// we should add a filter that allows storage providers to only return storage spaces the current user has access to
	//if spaceType == "home" && spaceName != "Shares" { // FIXME get rid of special /home/Shares folder. Might require testsuite changes ... a lot
	//	spaceType = "personal"
	//}

	for _, rule := range r.c.Rules {
		/*
			if strings.HasPrefix(ref.Path, path) {
				// we found a manual path config in the rules
				return []*registrypb.ProviderInfo{
					{
						Address:      rule.Address,
						ProviderPath: path,
					},
				}, nil
			}
		*/
		p := &registrypb.ProviderInfo{
			Address: rule.Address,
		}
		var spaces []*provider.StorageSpace
		var err error
		//if spaceType == "" {
		spaces, err = r.findStorageSpaceOnProviderByAccess(ctx, p, currentUser)
		//} else {
		//	spaces, err = r.findStorageSpaceOnProviderByType(ctx, p, spaceType) // TODO also filter by access
		//}
		if err == nil {
			for _, space := range spaces {
				p := &registrypb.ProviderInfo{
					ProviderId: space.Root.StorageId + "!" + space.Root.OpaqueId,
					Address:    rule.Address,
				}
				// cache entry
				name := space.Name
				if name == "" {
					name = space.Id.OpaqueId
				}
				p.ProviderPath = filepath.Join("/", space.SpaceType, name) // TODO deduplicate name
				/*r.*/ aliases /*[currentUser.Id.OpaqueId]*/ [p.ProviderPath] = &spaceAndProvider{
					space, []*registrypb.ProviderInfo{p},
				}
				// also register a personal storage where the current user is owner as his /home
				if space.SpaceType == "personal" && space.Owner != nil && utils.UserEqual(space.Owner.Id, currentUser.Id) {
					/*r.*/ aliases /*[currentUser.Id.OpaqueId]*/ ["/home"] = &spaceAndProvider{
						space, []*registrypb.ProviderInfo{{
							ProviderPath: "/home",
							ProviderId:   space.Root.StorageId + "!" + space.Root.OpaqueId,
							Address:      rule.Address,
						}},
					}
				}
				// also register a Shares storage containing all shares at /home/Shares
				// FIXME make mount point of Shares storageprovider configurable
				if space.SpaceType == "share" {
					alias := filepath.Join("/home/Shares", space.Name)
					/*r.*/ aliases /*[currentUser.Id.OpaqueId]*/ [alias] = &spaceAndProvider{
						space, []*registrypb.ProviderInfo{{
							ProviderPath: alias,
							ProviderId:   space.Root.StorageId + "!" + space.Root.OpaqueId,
							Address:      rule.Address,
						}},
					}
				}
				// cache result, TODO only for 30sec?
				//if _, ok := r.aliases[currentUser.Id.OpaqueId][p.ProviderPath]; !ok {
				/*} /*else {
					// add an additional storage provider, eg for load balancing
					r.aliases[currentUser.Id.OpaqueId][p.ProviderPath].providers = append(r.aliases[currentUser.Id.OpaqueId][p.ProviderPath].providers, p)
				}*/
			}

			/*
				if spaceAndAddr, ok := r.aliases[currentUser.Id.OpaqueId][alias]; ok {
					return spaceAndAddr.providers, nil
				}
			*/
		}
	}
	providers := make([]*registrypb.ProviderInfo, 0, len( /*r.*/ aliases /*[currentUser.Id.OpaqueId]*/))
	deepestMountPath := ""
	for mountPath, spaceAndProvider := range /*r.*/ aliases /*[currentUser.Id.OpaqueId]*/ {
		switch {
		case strings.HasPrefix(mountPath, ref.Path):
			// and add all providers below and exactly matching the path
			// requested /foo, mountPath /foo/sub
			providers = append(providers, spaceAndProvider.providers...)
		case strings.HasPrefix(ref.Path, mountPath) && len(mountPath) > len(deepestMountPath):
			// eg. three providers: /foo, /foo/sub, /foo/sub/bar
			// requested /foo/sub/mob
			deepestMountPath = mountPath
		}
	}
	if deepestMountPath != "" {
		providers = append(providers /*r.*/, aliases /*[currentUser.Id.OpaqueId]*/ [deepestMountPath].providers...)
	}
	return providers, nil
}

func (r *registry) findResourceOnProvider(ctx context.Context, p *registrypb.ProviderInfo, res *provider.ResourceId) (*provider.ResourceInfo, error) {
	c, err := pool.GetStorageProviderServiceClient(p.Address)
	if err != nil {
		return nil, err
	}
	req := &provider.StatRequest{
		Ref: &provider.Reference{ResourceId: res},
	}

	statResponse, err := c.Stat(ctx, req)
	if err != nil {
		return nil, err
	}
	if statResponse.Status.Code != rpc.Code_CODE_OK {
		return nil, status.NewErrorFromCode(statResponse.Status.Code, "spaces registry")
	}
	return statResponse.Info, nil
}

func (r *registry) findStorageSpaceOnProviderByAccess(ctx context.Context, p *registrypb.ProviderInfo, u *userv1beta1.User) ([]*provider.StorageSpace, error) {
	c, err := pool.GetStorageProviderServiceClient(p.Address)
	if err != nil {
		return nil, err
	}
	req := &provider.ListStorageSpacesRequest{
		// TODO
	}

	res, err := c.ListStorageSpaces(ctx, req)
	if err != nil {
		return nil, err
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		return nil, status.NewErrorFromCode(res.Status.Code, "spaces registry")
	}
	return res.StorageSpaces, nil
}

/*
func (r *registry) findStorageSpaceOnProviderByType(ctx context.Context, p *registrypb.ProviderInfo, spacetype string) ([]*provider.StorageSpace, error) {
	c, err := pool.GetStorageProviderServiceClient(p.Address)
	if err != nil {
		return nil, err
	}
	req := &provider.ListStorageSpacesRequest{
		Filters: []*provider.ListStorageSpacesRequest_Filter{{
			Type: provider.ListStorageSpacesRequest_Filter_TYPE_SPACE_TYPE,
			Term: &provider.ListStorageSpacesRequest_Filter_SpaceType{SpaceType: spacetype},
		}},
	}

	res, err := c.ListStorageSpaces(ctx, req)
	if err != nil {
		return nil, err
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		return nil, status.NewErrorFromCode(res.Status.Code, "spaces registry")
	}
	return res.StorageSpaces, nil
}

func (r *registry) findNameForRoot(ctx context.Context, p *registrypb.ProviderInfo, root *provider.ResourceId) (string, error) {
	if name, ok := r.resourceNameCache[root.StorageId+":"+root.OpaqueId]; ok {
		return name, nil
	}
	c, err := pool.GetStorageProviderServiceClient(p.Address)
	if err != nil {
		return "", err
	}

	req := &provider.StatRequest{
		Ref: &provider.Reference{ResourceId: root},
	}
	res, err := c.Stat(ctx, req)
	if err != nil {
		return "", err
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		return "", status.NewErrorFromCode(res.Status.Code, "spaces registry")
	}
	// TODO implement proper concurrency safe cache
	r.resourceNameCache[root.StorageId+":"+root.OpaqueId] = res.Info.Path
	return res.Info.Path, nil
}
*/
