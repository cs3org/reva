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
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	providerpb "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registrypb "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/logger"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/storage"
	pkgregistry "github.com/cs3org/reva/pkg/storage/registry/registry"
	"github.com/mitchellh/mapstructure"
	"google.golang.org/grpc"
)

//go:generate mockery -name StorageProviderClient

func init() {
	pkgregistry.Register("spaces", NewDefault)
}

type spaceConfig struct {
	// MountPoint determines where a space is mounted. Can be a regex
	// It is used to determine which storage provider is responsible when only a path is given in the request
	MountPoint string `mapstructure:"mount_point"`
	// PathTemplate is used to build the path of an individual space. Layouts can access {{.Space...}} and {{.CurrentUser...}}
	PathTemplate string `mapstructure:"path_template"`
	template     *template.Template
	// filters
	OwnerIsCurrentUser bool   `mapstructure:"owner_is_current_user"`
	ID                 string `mapstructure:"id"`
	// TODO description?
}

// SpacePath generates a layout based on space data.
func (sc *spaceConfig) SpacePath(currentUser *userpb.User, space *providerpb.StorageSpace) (string, error) {
	b := bytes.Buffer{}
	if err := sc.template.Execute(&b, templateData{CurrentUser: currentUser, Space: space}); err != nil {
		return "", err
	}
	return b.String(), nil
}

type provider struct {
	// Spaces is a map from space type to space config
	Spaces map[string]*spaceConfig `mapstructure:"spaces"`
}

type templateData struct {
	CurrentUser *userpb.User
	Space       *providerpb.StorageSpace
}

// StorageProviderClient is the interface the spaces registry uses to interact with storage providers
type StorageProviderClient interface {
	ListStorageSpaces(ctx context.Context, in *providerpb.ListStorageSpacesRequest, opts ...grpc.CallOption) (*providerpb.ListStorageSpacesResponse, error)
}

type config struct {
	Providers    map[string]*provider `mapstructure:"providers"`
	HomeTemplate string               `mapstructure:"home_template"`
}

func (c *config) init() {

	if c.HomeTemplate == "" {
		c.HomeTemplate = "/"
	}

	if len(c.Providers) == 0 {
		c.Providers = map[string]*provider{
			sharedconf.GetGatewaySVC(""): {
				Spaces: map[string]*spaceConfig{
					"personal":   {MountPoint: "/users", PathTemplate: "/users/{{.Space.Owner.Id.OpaqueId}}"},
					"project":    {MountPoint: "/projects", PathTemplate: "/projects/{{.Space.Name}}"},
					"virtual":    {MountPoint: "/users/{{.CurrentUser.Id.OpaqueId}}/Shares"},
					"grant":      {MountPoint: "."},
					"mountpoint": {MountPoint: "/users/{{.CurrentUser.Id.OpaqueId}}/Shares", PathTemplate: "/users/{{.CurrentUser.Id.OpaqueId}}/Shares/{{.Space.Name}}"},
					"public":     {MountPoint: "/public"},
				},
			},
		}
	}

	// cleanup space paths
	for _, provider := range c.Providers {
		for _, space := range provider.Spaces {

			if space.MountPoint == "" {
				space.MountPoint = "/"
			}

			// if the path template is not explicitly set use the mount point as path template
			if space.PathTemplate == "" {
				space.PathTemplate = space.MountPoint
			}

			// cleanup path templates
			space.PathTemplate = filepath.Join("/", space.PathTemplate)

			// compile given template tpl
			var err error
			space.template, err = template.New("path_template").Funcs(sprig.TxtFuncMap()).Parse(space.PathTemplate)
			if err != nil {
				logger.New().Fatal().Err(err).Interface("space", space).Msg("error parsing template")
			}
		}

		// TODO connect to provider, (List Spaces,) ListContainerStream
	}
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

// New creates an implementation of the storage.Registry interface that
// uses the available storage spaces from the configured storage providers
func New(m map[string]interface{}, getClientFunc GetStorageProviderServiceClientFunc) (storage.Registry, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	c.init()
	r := &registry{
		c:                               c,
		resources:                       make(map[string][]*registrypb.ProviderInfo),
		resourceNameCache:               make(map[string]string),
		getStorageProviderServiceClient: getClientFunc,
	}
	r.homeTemplate, err = template.New("home_template").Funcs(sprig.TxtFuncMap()).Parse(c.HomeTemplate)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// NewDefault creates an implementation of the storage.Registry interface that
// uses the available storage spaces from the configured storage providers
func NewDefault(m map[string]interface{}) (storage.Registry, error) {
	getClientFunc := func(addr string) (StorageProviderClient, error) {
		return pool.GetStorageProviderServiceClient(addr)
	}
	return New(m, getClientFunc)
}

// GetStorageProviderServiceClientFunc is a callback used to pass in a StorageProviderClient during testing
type GetStorageProviderServiceClientFunc func(addr string) (StorageProviderClient, error)

type registry struct {
	c *config
	// the template to use when determining the home provider
	homeTemplate *template.Template
	// a map of resources to providers
	resources         map[string][]*registrypb.ProviderInfo
	resourceNameCache map[string]string

	getStorageProviderServiceClient GetStorageProviderServiceClientFunc
}

// GetProvider return the storage provider for the given spaces according to the rule configuration
func (r *registry) GetProvider(ctx context.Context, space *providerpb.StorageSpace) (*registrypb.ProviderInfo, error) {
	for address, provider := range r.c.Providers {
		for spaceType, sc := range provider.Spaces {
			spacePath := ""
			var err error
			if space.SpaceType != "" && spaceType != space.SpaceType {
				continue
			}
			if space.Owner != nil {
				spacePath, err = sc.SpacePath(nil, space)
				if err != nil {
					continue
				}
				match, err := regexp.MatchString(sc.MountPoint, spacePath)
				if err != nil {
					continue
				}
				if !match {
					continue
				}
			}
			pi := &registrypb.ProviderInfo{Address: address}
			opaque, err := spacePathsToOpaque(map[string]string{"unused": spacePath})
			if err != nil {
				appctx.GetLogger(ctx).Debug().Err(err).Msg("marshaling space paths map failed, continuing")
				continue
			}
			pi.Opaque = opaque
			return pi, nil // return the first match we find
		}
	}
	return nil, errtypes.NotFound("no provider found for space")
}

// FIXME the config takes the mount path of a provider as key,
// - it will always be used as the Providerpath
// - if the mount path is a regex, the provider config needs a providerpath config that is used instead of the regex
// - the gateway ALWAYS replaces the mountpath with the spaceid? and builds a relative reference which is forwarded to the responsible provider

// FindProviders will return all providers that need to be queried for a request
// - for an id based or relative request it will return the providers that serve the storage space
// - for a path based request it will return the provider with the most specific mount path, as
//   well as all spaces mounted below the requested path. Stat and ListContainer requests need
//   to take their etag/mtime into account.
// The list of providers also contains the space that should be used as the root for the relative path
//
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
func (r *registry) ListProviders(ctx context.Context, filters map[string]string) ([]*registrypb.ProviderInfo, error) {
	switch {
	case filters["storage_id"] != "" && filters["opaque_id"] != "":
		findMountpoint := filters["type"] == "mountpoint"
		findGrant := !findMountpoint && filters["path"] == "" // relvative references, by definition, occur in the correct storage, so do not look for grants
		return r.findProvidersForResource(ctx, filters["storage_id"]+"!"+filters["opaque_id"], findMountpoint, findGrant), nil
	case filters["path"] != "":
		return r.findProvidersForAbsolutePathReference(ctx, filters["path"]), nil
		// TODO add filter for all spaces the user can manage?
	case len(filters) == 0:
		// return all providers
		return r.findAllProviders(ctx), nil
	}
	return []*registrypb.ProviderInfo{}, nil
}

// findProvidersForResource looks up storage providers based on a resource id
// for the root of a space the res.StorageId is the same as the res.OpaqueId
// for share spaces the res.StorageId tells the registry the spaceid and res.OpaqueId is a node in that space
func (r *registry) findProvidersForResource(ctx context.Context, id string, findMoundpoint, findGrant bool) []*registrypb.ProviderInfo {
	currentUser := ctxpkg.ContextMustGetUser(ctx)
	providerInfos := []*registrypb.ProviderInfo{}
	for address, provider := range r.c.Providers {
		p := &registrypb.ProviderInfo{
			Address:    address,
			ProviderId: id,
		}
		filters := []*providerpb.ListStorageSpacesRequest_Filter{{
			Type: providerpb.ListStorageSpacesRequest_Filter_TYPE_ID,
			Term: &providerpb.ListStorageSpacesRequest_Filter_Id{
				Id: &providerpb.StorageSpaceId{
					OpaqueId: id,
				},
			},
		}}
		if findMoundpoint {
			// when listing by id return also grants and mountpoints
			filters = append(filters, &providerpb.ListStorageSpacesRequest_Filter{
				Type: providerpb.ListStorageSpacesRequest_Filter_TYPE_SPACE_TYPE,
				Term: &providerpb.ListStorageSpacesRequest_Filter_SpaceType{
					SpaceType: "+mountpoint",
				},
			})
		}
		if findGrant {
			// when listing by id return also grants and mountpoints
			filters = append(filters, &providerpb.ListStorageSpacesRequest_Filter{
				Type: providerpb.ListStorageSpacesRequest_Filter_TYPE_SPACE_TYPE,
				Term: &providerpb.ListStorageSpacesRequest_Filter_SpaceType{
					SpaceType: "+grant",
				},
			})
		}
		spaces, err := r.findStorageSpaceOnProvider(ctx, address, filters)
		if err != nil {
			appctx.GetLogger(ctx).Debug().Err(err).Interface("provider", provider).Msg("findStorageSpaceOnProvider by id failed, continuing")
			continue
		}

		switch len(spaces) {
		case 0:
			// nothing to do, will continue with next provider
		case 1:
			space := spaces[0]
			var sc *spaceConfig
			var ok bool
			var spacePath string

			if space.SpaceType == "grant" {
				spacePath = "." // a . indicates a grant, the gateway will do a findMountpoint for it
			} else {
				if findMoundpoint && space.SpaceType != "mountpoint" {
					continue
				}
				// filter unwanted space types. type mountpoint is not explicitly configured but requested by the gateway
				if sc, ok = provider.Spaces[space.SpaceType]; !ok && space.SpaceType != "mountpoint" {
					continue
				}

				spacePath, err = sc.SpacePath(currentUser, space)
				if err != nil {
					appctx.GetLogger(ctx).Error().Err(err).Interface("provider", provider).Interface("space", space).Msg("failed to execute template, continuing")
					continue
				}
			}

			spacePaths := map[string]string{
				space.Id.OpaqueId: spacePath,
			}
			p.Opaque, err = spacePathsToOpaque(spacePaths)
			if err != nil {
				appctx.GetLogger(ctx).Debug().Err(err).Msg("marshaling space paths map failed, continuing")
				continue
			}
			// we can stop after we found the first space
			// TODO to improve lookup time the registry could cache which provider last was responsible for a space? could be invalidated by simple ttl? would that work for shares?
			//return []*registrypb.ProviderInfo{p}
			providerInfos = append(providerInfos, p) // hm we need to query all providers ... or the id based lookup might only see the spaces storage provider
		default:
			// there should not be multiple spaces with the same id per provider
			appctx.GetLogger(ctx).Error().Err(err).Interface("provider", provider).Interface("spaces", spaces).Msg("multiple spaces returned, ignoring")
		}
	}
	return providerInfos
}

// findProvidersForAbsolutePathReference takes a path and returns the storage provider with the longest matching path prefix
// FIXME use regex to return the correct provider when multiple are configured
func (r *registry) findProvidersForAbsolutePathReference(ctx context.Context, path string) []*registrypb.ProviderInfo {
	currentUser := ctxpkg.ContextMustGetUser(ctx)

	deepestMountPath := ""
	var deepestMountSpace *providerpb.StorageSpace
	var deepestMountPathProvider *registrypb.ProviderInfo
	providers := map[string]map[string]string{}
	for address, provider := range r.c.Providers {
		p := &registrypb.ProviderInfo{
			Address: address,
		}
		var spaces []*providerpb.StorageSpace
		var err error
		filters := []*providerpb.ListStorageSpacesRequest_Filter{}
		// when listing paths also return mountpoints
		filters = append(filters, &providerpb.ListStorageSpacesRequest_Filter{
			Type: providerpb.ListStorageSpacesRequest_Filter_TYPE_SPACE_TYPE,
			Term: &providerpb.ListStorageSpacesRequest_Filter_SpaceType{
				SpaceType: "+mountpoint",
			},
		})

		spaces, err = r.findStorageSpaceOnProvider(ctx, p.Address, filters)
		if err != nil {
			appctx.GetLogger(ctx).Debug().Err(err).Interface("provider", provider).Msg("findStorageSpaceOnProvider failed, continuing")
			continue
		}

		spacePaths := map[string]string{}
		for _, space := range spaces {
			var sc *spaceConfig
			var ok bool

			if space.SpaceType == "grant" {
				spacePaths[space.Id.OpaqueId] = "." // a . indicates a grant, the gateway will do a findMountpoint for it
				continue
			}

			// filter unwanted space types. type mountpoint is not explicitly configured but requested by the gateway
			if sc, ok = provider.Spaces[space.SpaceType]; !ok {
				continue
			}
			spacePath, err := sc.SpacePath(currentUser, space)
			if err != nil {
				appctx.GetLogger(ctx).Error().Err(err).Interface("provider", provider).Interface("space", space).Msg("failed to execute template, continuing")
				continue
			}

			switch {
			case strings.HasPrefix(spacePath, path):
				// and add all providers below and exactly matching the path
				// requested /foo, mountPath /foo/sub
				spacePaths[space.Id.OpaqueId] = spacePath
				if len(spacePath) > len(deepestMountPath) {
					deepestMountPath = spacePath
					deepestMountSpace = space
					deepestMountPathProvider = p
				}
			case strings.HasPrefix(path, spacePath) && len(spacePath) > len(deepestMountPath):
				// eg. three providers: /foo, /foo/sub, /foo/sub/bar
				// requested /foo/sub/mob
				deepestMountPath = spacePath
				deepestMountSpace = space
				deepestMountPathProvider = p
			}
		}

		if len(spacePaths) > 0 {
			providers[p.Address] = spacePaths
		}
	}

	if deepestMountPathProvider != nil {
		if spacePaths, ok := providers[deepestMountPathProvider.Address]; ok {
			spacePaths[deepestMountSpace.Id.OpaqueId] = deepestMountPath
		} else {
			providers[deepestMountPathProvider.Address] = map[string]string{deepestMountSpace.Id.OpaqueId: deepestMountPath}
		}
	}

	pis := make([]*registrypb.ProviderInfo, 0, len(providers))
	for addr, spacePaths := range providers {
		pi := &registrypb.ProviderInfo{Address: addr}
		opaque, err := spacePathsToOpaque(spacePaths)
		if err != nil {
			appctx.GetLogger(ctx).Debug().Err(err).Msg("marshaling space paths map failed, continuing")
			continue
		}
		pi.Opaque = opaque
		pis = append(pis, pi)
	}

	return pis
}

// findAllProviders returns a list of all storage providers
// This is a dumb call that does not call ListStorageSpaces() on the providers: ListStorageSpaces() in the gateway can cache that better.
func (r *registry) findAllProviders(ctx context.Context) []*registrypb.ProviderInfo {
	pis := make([]*registrypb.ProviderInfo, 0, len(r.c.Providers))
	for address := range r.c.Providers {
		pis = append(pis, &registrypb.ProviderInfo{
			Address: address,
		})
	}
	return pis
}

func spacePathsToOpaque(spacePaths map[string]string) (*typesv1beta1.Opaque, error) {
	spacePathsJSON, err := json.Marshal(spacePaths)
	if err != nil {
		return nil, err
	}
	return &typesv1beta1.Opaque{
		Map: map[string]*typesv1beta1.OpaqueEntry{
			"space_paths": {
				Decoder: "json",
				Value:   spacePathsJSON,
			},
		},
	}, nil
}

func (r *registry) findStorageSpaceOnProvider(ctx context.Context, addr string, filters []*providerpb.ListStorageSpacesRequest_Filter) ([]*providerpb.StorageSpace, error) {
	c, err := r.getStorageProviderServiceClient(addr)
	if err != nil {
		return nil, err
	}
	req := &providerpb.ListStorageSpacesRequest{
		Filters: filters,
	}

	res, err := c.ListStorageSpaces(ctx, req)
	if err != nil {
		return nil, err
	}
	if res.Status.Code != rpc.Code_CODE_OK && res.Status.Code != rpc.Code_CODE_NOT_FOUND {
		return nil, status.NewErrorFromCode(res.Status.Code, "spaces registry")
	}
	return res.StorageSpaces, nil
}
