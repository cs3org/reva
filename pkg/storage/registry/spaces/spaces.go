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

type provider struct {
	Mapping           string   `mapstructure:"mapping"`
	MountPath         string   `mapstructure:"mount_path"`
	AllowedUserAgents []string `mapstructure:"allowed_user_agents"`
	PathTemplate      string   `mapstructure:"path_template"`
	template          *template.Template
	// filters
	SpaceType      string `mapstructure:"space_type"`
	SpaceOwnerSelf bool   `mapstructure:"space_owner_self"`
	SpaceID        string `mapstructure:"space_id"`
}

type templateData struct {
	CurrentUser *userpb.User
	Space       *providerpb.StorageSpace
}

// StorageProviderClient is the interface the spaces registry uses to interact with storage providers
type StorageProviderClient interface {
	ListStorageSpaces(ctx context.Context, in *providerpb.ListStorageSpacesRequest, opts ...grpc.CallOption) (*providerpb.ListStorageSpacesResponse, error)
}

// WithSpace generates a layout based on space data.
func (p *provider) ProviderPath(u *userpb.User, s *providerpb.StorageSpace) (string, error) {
	b := bytes.Buffer{}
	if err := p.template.Execute(&b, templateData{CurrentUser: u, Space: s}); err != nil {
		return "", err
	}
	return b.String(), nil
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
				MountPath: "/",
			},
		}
	}

	// cleanup provider paths
	for _, provider := range c.Providers {
		// if the path template is not explicitly set use the mountpath as path template
		if provider.PathTemplate == "" && strings.HasPrefix(provider.MountPath, "/") {
			// TODO err if the path is a regex
			provider.PathTemplate = provider.MountPath
		}

		// cleanup path template
		provider.PathTemplate = filepath.Clean(provider.PathTemplate)

		// compile given template tpl
		var err error
		provider.template, err = template.New("path_template").Funcs(sprig.TxtFuncMap()).Parse(provider.PathTemplate)
		if err != nil {
			logger.New().Fatal().Err(err).Interface("provider", provider).Msg("error parsing template")
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
	for address, rule := range r.c.Providers {
		mountPath := ""
		var err error
		if space.SpaceType != "" && rule.SpaceType != space.SpaceType {
			continue
		}
		if space.Owner != nil {
			mountPath, err = rule.ProviderPath(nil, space)
			if err != nil {
				continue
			}
			match, err := regexp.MatchString(rule.MountPath, mountPath)
			if err != nil {
				continue
			}
			if !match {
				continue
			}
		}
		pi := &registrypb.ProviderInfo{Address: address}
		opaque, err := spacePathsToOpaque(map[string]string{"unused": mountPath})
		if err != nil {
			appctx.GetLogger(ctx).Debug().Err(err).Msg("marshaling space paths map failed, continuing")
			continue
		}
		pi.Opaque = opaque
		return pi, nil
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
		return r.findProvidersForResource(ctx, filters["storage_id"]+"!"+filters["opaque_id"]), nil
	case filters["path"] != "":
		return r.findProvidersForAbsolutePathReference(ctx, filters["path"]), nil
	}
	return []*registrypb.ProviderInfo{}, nil
}

// findProvidersForResource looks up storage providers based on a resource id
// for the root of a space the res.StorageId is the same as the res.OpaqueId
// for share spaces the res.StorageId tells the registry the spaceid and res.OpaqueId is a node in that space
func (r *registry) findProvidersForResource(ctx context.Context, id string) []*registrypb.ProviderInfo {
	currentUser := ctxpkg.ContextMustGetUser(ctx)
	for address, rule := range r.c.Providers {
		p := &registrypb.ProviderInfo{
			Address:    address,
			ProviderId: id,
		}
		filters := []*providerpb.ListStorageSpacesRequest_Filter{}
		if rule.SpaceType != "" {
			// add filter to id based request if it is configured
			filters = append(filters, &providerpb.ListStorageSpacesRequest_Filter{
				Type: providerpb.ListStorageSpacesRequest_Filter_TYPE_SPACE_TYPE,
				Term: &providerpb.ListStorageSpacesRequest_Filter_SpaceType{
					SpaceType: rule.SpaceType,
				},
			})
		}
		filters = append(filters, &providerpb.ListStorageSpacesRequest_Filter{
			Type: providerpb.ListStorageSpacesRequest_Filter_TYPE_ID,
			Term: &providerpb.ListStorageSpacesRequest_Filter_Id{
				Id: &providerpb.StorageSpaceId{
					OpaqueId: id,
				},
			},
		})
		spaces, err := r.findStorageSpaceOnProvider(ctx, address, filters)
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
	var deepestMountSpace *providerpb.StorageSpace
	var deepestMountPathProvider *registrypb.ProviderInfo
	providers := map[string]map[string]string{}
	for address, rule := range r.c.Providers {
		p := &registrypb.ProviderInfo{
			Address: address,
		}
		var spaces []*providerpb.StorageSpace
		var err error
		filters := []*providerpb.ListStorageSpacesRequest_Filter{}
		if rule.SpaceOwnerSelf {
			filters = append(filters, &providerpb.ListStorageSpacesRequest_Filter{
				Type: providerpb.ListStorageSpacesRequest_Filter_TYPE_OWNER,
				Term: &providerpb.ListStorageSpacesRequest_Filter_Owner{
					Owner: currentUser.Id,
				},
			})
		}
		if rule.SpaceType != "" {
			filters = append(filters, &providerpb.ListStorageSpacesRequest_Filter{
				Type: providerpb.ListStorageSpacesRequest_Filter_TYPE_SPACE_TYPE,
				Term: &providerpb.ListStorageSpacesRequest_Filter_SpaceType{
					SpaceType: rule.SpaceType,
				},
			})
		}
		if rule.SpaceID != "" {
			filters = append(filters, &providerpb.ListStorageSpacesRequest_Filter{
				Type: providerpb.ListStorageSpacesRequest_Filter_TYPE_ID,
				Term: &providerpb.ListStorageSpacesRequest_Filter_Id{
					Id: &providerpb.StorageSpaceId{OpaqueId: rule.SpaceID},
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
		if err != nil {
			appctx.GetLogger(ctx).Debug().Err(err).Msg("marshaling space paths map failed, continuing")
			continue
		}
		pi.Opaque = opaque
		pis = append(pis, pi)
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
	if res.Status.Code != rpc.Code_CODE_OK {
		return nil, status.NewErrorFromCode(res.Status.Code, "spaces registry")
	}
	return res.StorageSpaces, nil
}
