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
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
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

type rule struct {
	Mapping           string            `mapstructure:"mapping"`
	Address           string            `mapstructure:"address"`
	Aliases           map[string]string `mapstructure:"aliases"`
	AllowedUserAgents []string          `mapstructure:"allowed_user_agents"`
	PathTemplate      string            `mapstructure:"path_template"`
	template          *template.Template
	// filters
	SpaceType      string `mapstructure:"space_type"`
	SpaceOwnerSelf bool   `mapstructure:"space_owner_self"`
	SpaceID        string `mapstructure:"space_id"`
}

type templateData struct {
	CurrentUser *userpb.User
	Space       *provider.StorageSpace
}

type StorageProviderClient interface {
	ListStorageSpaces(ctx context.Context, in *provider.ListStorageSpacesRequest, opts ...grpc.CallOption) (*provider.ListStorageSpacesResponse, error)
}

// WithSpace generates a layout based on space data.
func (r *rule) ProviderPath(u *userpb.User, s *provider.StorageSpace) (string, error) {
	b := bytes.Buffer{}
	if err := r.template.Execute(&b, templateData{CurrentUser: u, Space: s}); err != nil {
		return "", err
	}
	return b.String(), nil
}

type config struct {
	Rules        map[string]*rule `mapstructure:"rules"`
	HomeTemplate string           `mapstructure:"home_template"`
}

func (c *config) init() {

	if c.HomeTemplate == "" {
		c.HomeTemplate = "/"
	}

	if len(c.Rules) == 0 {
		c.Rules = map[string]*rule{
			"/": {
				Address: sharedconf.GetGatewaySVC(""),
			},
			"00000000-0000-0000-0000-000000000000": {
				Address: sharedconf.GetGatewaySVC(""),
			},
		}
	}

	// cleanup rule paths
	for path, rule := range c.Rules {
		// if the path template is not explicitly set use the key as path template
		if rule.PathTemplate == "" && strings.HasPrefix(path, "/") {
			// TODO err if the path is a regex
			rule.PathTemplate = path
		}

		// cleanup path template
		rule.PathTemplate = filepath.Clean(rule.PathTemplate)

		// compile given template tpl
		var err error
		rule.template, err = template.New("path_template").Funcs(sprig.TxtFuncMap()).Parse(rule.PathTemplate)
		if err != nil {
			logger.New().Fatal().Err(err).Interface("rule", rule).Msg("error parsing template")
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
func New(m map[string]interface{}, getClientFunc GetStorageProviderServiceClientFunc) (storage.Registry, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	c.init()
	r := &registry{
		c:         c,
		resources: make(map[string][]*registrypb.ProviderInfo),
		//aliases:           make(map[string]map[string]*spaceAndProvider),
		resourceNameCache:               make(map[string]string),
		getStorageProviderServiceClient: getClientFunc,
	}
	r.homeTemplate, err = template.New("home_template").Funcs(sprig.TxtFuncMap()).Parse(c.HomeTemplate)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func NewDefault(m map[string]interface{}) (storage.Registry, error) {
	getClientFunc := func(addr string) (StorageProviderClient, error) {
		return pool.GetStorageProviderServiceClient(addr)
	}
	return New(m, getClientFunc)
}

type spaceAndProvider struct {
	*provider.StorageSpace
	providers []*registrypb.ProviderInfo
}

type GetStorageProviderServiceClientFunc func(addr string) (StorageProviderClient, error)

type registry struct {
	c *config
	// the template to use when determining the home provider
	homeTemplate *template.Template
	// a map of resources to providers
	resources map[string][]*registrypb.ProviderInfo
	// a map of paths/aliases to spaces and providers
	// aliases           map[string]map[string]*spaceAndProvider
	resourceNameCache map[string]string

	getStorageProviderServiceClient GetStorageProviderServiceClientFunc
}

// GetProvider return the storage provider for the given spaces according to the rule configuration
func (r *registry) GetProvider(ctx context.Context, space *provider.StorageSpace) (*registrypb.ProviderInfo, error) {
	for pattern, rule := range r.c.Rules {
		if space.SpaceType != "" && rule.SpaceType != space.SpaceType {
			continue
		}
		if space.Owner != nil {
			providerPath, err := rule.ProviderPath(nil, space)
			if err != nil {
				continue
			}
			match, err := regexp.MatchString(pattern, providerPath)
			if err != nil {
				continue
			}
			if !match {
				continue
			}
		}
		return &registrypb.ProviderInfo{
			Address: rule.Address,
		}, nil
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
	if filters["path"] != "" {
		return r.findProvidersForAbsolutePathReference(ctx, filters["path"]), nil
	} else if filters["storage_id"] != "" && filters["opaque_id"] != "" {
		return r.findProvidersForResource(ctx, filters["storage_id"]+"!"+filters["opaque_id"]), nil
	}

	// switch {
	// case ref.ResourceId != nil && ref.ResourceId.StorageId != "":
	// 	return r.findProvidersForResource(ctx, ref.ResourceId)
	// case utils.IsAbsolutePathReference(ref):
	// 	return r.findProvidersForAbsolutePathReference(ctx, ref)
	// default:
	// 	return nil, errtypes.NotSupported("unsupported reference type")
	// }
	return []*registrypb.ProviderInfo{}, nil
}

// spaceID is a workaround te glue together a spaceid that can carry both: the spaceid AND the nodeid
// the spaceid is needed for routing in the gateway AND for finding the correct storage space in the rpovider
// the nodeid is needed by the provider to find the shared node
func spaceID(res *provider.ResourceId) string {
	return res.StorageId + "!" + res.OpaqueId
}

// findProvidersForResource looks up storage providers based on a resource id
// for the root of a space the res.StorageId is the same as the res.OpaqueId
// for share spaces the res.StorageId tells the registry the spaceid and res.OpaqueId is a node in that space
func (r *registry) findProvidersForResource(ctx context.Context, id string) []*registrypb.ProviderInfo {
	currentUser := ctxpkg.ContextMustGetUser(ctx)
	for _, rule := range r.c.Rules {
		p := &registrypb.ProviderInfo{
			Address:    rule.Address,
			ProviderId: id,
		}
		filters := []*provider.ListStorageSpacesRequest_Filter{}
		if rule.SpaceType != "" {
			// add filter to id based request if it is configured
			filters = append(filters, &provider.ListStorageSpacesRequest_Filter{
				Type: provider.ListStorageSpacesRequest_Filter_TYPE_SPACE_TYPE,
				Term: &provider.ListStorageSpacesRequest_Filter_SpaceType{
					SpaceType: rule.SpaceType,
				},
			})
		}
		filters = append(filters, &provider.ListStorageSpacesRequest_Filter{
			Type: provider.ListStorageSpacesRequest_Filter_TYPE_ID,
			Term: &provider.ListStorageSpacesRequest_Filter_Id{
				Id: &provider.StorageSpaceId{
					OpaqueId: id,
				},
			},
		})
		spaces, err := r.findStorageSpaceOnProvider(ctx, rule.Address, filters)
		if err != nil {
			appctx.GetLogger(ctx).Debug().Err(err).Interface("rule", rule).Msg("findStorageSpaceOnProvider by id failed, continuing")
			continue
		}

		if len(spaces) > 0 {
			space := spaces[0] // there shouldn't be multiple
			providerPath, err := rule.ProviderPath(currentUser, space)
			if err != nil {
				appctx.GetLogger(ctx).Error().Err(err).Interface("rule", rule).Interface("space", space).Msg("failed to execute template, continuing")
				continue
			}

			spacePaths := map[string]string{
				space.Id.OpaqueId: providerPath,
			}
			p.Opaque, err = spacePathsToOpaque(spacePaths)
			if err != nil {
				appctx.GetLogger(ctx).Debug().Err(err).Msg("marshaling space paths map failed, continuing")
				continue
			}
			return []*registrypb.ProviderInfo{p}
		}
	}
	return []*registrypb.ProviderInfo{}
}

// findProvidersForAbsolutePathReference takes a path and ruturns the storage provider with the longest matching path prefix
// FIXME use regex to return the correct provider when multiple are configured
func (r *registry) findProvidersForAbsolutePathReference(ctx context.Context, path string) []*registrypb.ProviderInfo {
	currentUser := ctxpkg.ContextMustGetUser(ctx)

	deepestMountPath := ""
	var deepestMountSpace *provider.StorageSpace
	var deepestMountPathProvider *registrypb.ProviderInfo
	providers := map[string]map[string]string{}
	for _, rule := range r.c.Rules {
		p := &registrypb.ProviderInfo{
			Address: rule.Address,
		}
		var spaces []*provider.StorageSpace
		var err error
		filters := []*provider.ListStorageSpacesRequest_Filter{}
		if rule.SpaceOwnerSelf {
			filters = append(filters, &provider.ListStorageSpacesRequest_Filter{
				Type: provider.ListStorageSpacesRequest_Filter_TYPE_OWNER,
				Term: &provider.ListStorageSpacesRequest_Filter_Owner{
					Owner: currentUser.Id,
				},
			})
		}
		if rule.SpaceType != "" {
			filters = append(filters, &provider.ListStorageSpacesRequest_Filter{
				Type: provider.ListStorageSpacesRequest_Filter_TYPE_SPACE_TYPE,
				Term: &provider.ListStorageSpacesRequest_Filter_SpaceType{
					SpaceType: rule.SpaceType,
				},
			})
		}
		if rule.SpaceID != "" {
			filters = append(filters, &provider.ListStorageSpacesRequest_Filter{
				Type: provider.ListStorageSpacesRequest_Filter_TYPE_ID,
				Term: &provider.ListStorageSpacesRequest_Filter_Id{
					Id: &provider.StorageSpaceId{OpaqueId: rule.SpaceID},
				},
			})
		}

		spaces, err = r.findStorageSpaceOnProvider(ctx, p.Address, filters)
		if err != nil {
			appctx.GetLogger(ctx).Debug().Err(err).Interface("rule", rule).Msg("findStorageSpaceOnProvider failed, continuing")
			continue
		}

		spacePaths := map[string]string{}
		for _, space := range spaces {
			spacePath, err := rule.ProviderPath(currentUser, space)
			if err != nil {
				appctx.GetLogger(ctx).Error().Err(err).Interface("rule", rule).Interface("space", space).Msg("failed to execute template, continuing")
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
		pi.Opaque = opaque
		if err != nil {
			appctx.GetLogger(ctx).Debug().Err(err).Msg("marshaling space paths map failed, continuing")
			continue
		}
		pis = append(pis, pi)
	}

	return pis
}

func spacePathsToOpaque(spacePaths map[string]string) (*typesv1beta1.Opaque, error) {
	spacePathsJson, err := json.Marshal(spacePaths)
	if err != nil {
		return nil, err
	}
	return &typesv1beta1.Opaque{
		Map: map[string]*typesv1beta1.OpaqueEntry{
			"space_paths": {
				Decoder: "json",
				Value:   spacePathsJson,
			},
		},
	}, nil
}

func (r *registry) findStorageSpaceOnProvider(ctx context.Context, addr string, filters []*provider.ListStorageSpacesRequest_Filter) ([]*provider.StorageSpace, error) {
	c, err := r.getStorageProviderServiceClient(addr)
	if err != nil {
		return nil, err
	}
	req := &provider.ListStorageSpacesRequest{
		Filters: filters,
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
