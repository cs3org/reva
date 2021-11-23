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
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registrypb "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/logger"
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
func New(m map[string]interface{}) (storage.Registry, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	c.init()
	r := &registry{
		c:         c,
		resources: make(map[string][]*registrypb.ProviderInfo),
		//aliases:           make(map[string]map[string]*spaceAndProvider),
		resourceNameCache: make(map[string]string),
	}
	r.homeTemplate, err = template.New("home_template").Funcs(sprig.TxtFuncMap()).Parse(c.HomeTemplate)
	if err != nil {
		return nil, err
	}
	return r, nil
}

type spaceAndProvider struct {
	*provider.StorageSpace
	providers []*registrypb.ProviderInfo
}

type registry struct {
	c *config
	// the template to use when determining the home provider
	homeTemplate *template.Template
	// a map of resources to providers
	resources map[string][]*registrypb.ProviderInfo
	// a map of paths/aliases to spaces and providers
	// aliases           map[string]map[string]*spaceAndProvider
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
				ProviderId:   spaceID(space.Root),
				ProviderPath: filepath.Join("/", space.SpaceType, space.Name), // TODO do we need to guarantee these are unique?
				Address:      rule.Address,
			}
			providers = append(providers, pi)
			r.resources[spaceID(space.Root)] = []*registrypb.ProviderInfo{pi}
		}
	}
	return providers, nil
}

// GetHome is called by the gateway to determine the address of the storage provider that should
// be uset to make a CreateHome call. It does not need to return a path or id. Only the address is used.
// In the spaces registry we will look up a rule matching the configured home template
func (r *registry) GetHome(ctx context.Context) (*registrypb.ProviderInfo, error) {
	currentUser := ctxpkg.ContextMustGetUser(ctx)
	b := bytes.Buffer{}
	// TODO test template on startup
	if err := r.homeTemplate.Execute(&b, currentUser); err != nil {
		return nil, err
	}
	homePath := b.String()

	for pattern, rule := range r.c.Rules {
		if ok, err := regexp.MatchString(pattern, homePath); ok {
			return &registrypb.ProviderInfo{
				Address:      rule.Address,
				ProviderPath: homePath,
			}, nil
		} else if err != nil {
			appctx.GetLogger(ctx).Error().Err(err).Interface("rule", rule).Interface("pattern", pattern).Msg("invalid patter, skipping")
			continue
		}
	}

	return nil, errtypes.NotFound("no pattern matching " + homePath)
}

// FIXME the config takes the mount path of a provider as key,
// - it will always be used as the Providerpath
// - if the mount path is a regex, the provider config needs a providerpath config that is used instead of the regex
// - the gateway ALWAYS replaces the mountpath with the spaceid? and builds a relative reference which is forwarded to the responsible provider

// FindProviders will return all providers that need to be queried for a request
// - for an id based or relative request it will return the providers that serve the storage space
// - for a path based request it will return the provider with the most specific mount path, as
//   well as all spaces mountad below the requested path. Stat and ListContainer requests need
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

// spaceID is a workaround te glue together a spaceid that can carry both: the spaceid AND the nodeid
// the spaceid is needed for routing in the gateway AND for finding the correct storage space in the rpovider
// the nodeid is needed by the provider to find the shared node
func spaceID(res *provider.ResourceId) string {
	return res.StorageId + "!" + res.OpaqueId
}

// findProvidersForResource looks up storage providers based on a resource id
// for the root of a space the res.StorageId is the same as the res.OpaqueId
// for share spaces the res.StorageId tells the registry the spaceid and res.OpaqueId is a node in that space
func (r *registry) findProvidersForResource(ctx context.Context, res *provider.ResourceId) ([]*registrypb.ProviderInfo, error) {
	currentUser := ctxpkg.ContextMustGetUser(ctx)
	providers := []*registrypb.ProviderInfo{}
	for _, rule := range r.c.Rules {
		p := &registrypb.ProviderInfo{
			Address:    rule.Address,
			ProviderId: spaceID(res),
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
					OpaqueId: spaceID(res),
				},
			},
		})
		spaces, err := r.findStorageSpaceOnProvider(ctx, rule.Address, filters)
		if err != nil {
			appctx.GetLogger(ctx).Debug().Err(err).Interface("rule", rule).Msg("findStorageSpaceOnProvider by id failed, continuing")
			continue
		}

		for _, space := range spaces {
			p.ProviderPath, err = rule.ProviderPath(currentUser, space)
			if err != nil {
				appctx.GetLogger(ctx).Error().Err(err).Interface("rule", rule).Interface("space", space).Msg("failed to execute template, continuing")
				continue
			}

			providers = append(providers, p)
		}
	}
	if len(providers) == 0 {
		return nil, errtypes.NotFound("spaces registry: storage provider not found for reference:" + res.String())
	}
	return providers, nil
}

// findProvidersForAbsolutePathReference takes a path and ruturns the storage provider with the longest matching path prefix
// FIXME use regex to return the correct provider when multiple are configured
func (r *registry) findProvidersForAbsolutePathReference(ctx context.Context, ref *provider.Reference) ([]*registrypb.ProviderInfo, error) {
	currentUser := ctxpkg.ContextMustGetUser(ctx)
	aliases := map[string]*spaceAndProvider{}

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

		for _, space := range spaces {
			p := &registrypb.ProviderInfo{
				ProviderId: spaceID(space.Root), // The registry uses this to build the root resourceID for the relative request to the provider
				Address:    rule.Address,
			}
			// cache entry
			// TODO the name should not be taken from the space.Name property. That is a displayname.
			//      For the file listing the path segment we need another human readable unique identifier.
			//      Is an example take /users/{space-alias}, where `space-alias` is either:
			//      - a displayname: 'Albert Eintsein',
			//      - a user id: 'f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c'
			//      - a username: 'einstein'
			//      The latter is human readable, hopefully unique (for a directory).
			//      'Hopefully', because it is not collision free but we can add a numbered suffix
			//      to make it unique and users need to be able to rename them anyway.
			//      -> return an alias/pathsegment/filename property for spaces that is unique per user
			//         - allows users to alias a resource individually
			//         - allows storing that metadata in the filesystem also for indexing
			//         - what about cs3 references ... arent they a better solution?
			//           - they could be used by a home storage provider
			// Which parts of the namespace are admin defined and which are defined by users?
			// - The admin defines the mount points of spaces:
			//      /home = users home
			//         - here, the user is jailed into his personal storage space. The next path segment is already in it.
			//      /shares or /home/Shares = user shares
			//         - here the sharesstorageprovider is responsible for the next path segment
			//         - the registry returns a list of all spaces of type share?
			//         - this is where an alias as part of the space would be great because the gateway
			//           could use it to build the file listing for /home/Shares
			//      /project/alice = a project space for alice which is provided by a single storage space
			//      /spaces = a list of work spaces the user has access to
			//         - this is again the case where the next path segment should be human readable. Why?
			//         - the registry
			// We could add the type of spaces to list under a configured path to the rules:
			// - then configuring /users to list all spaces of type 'personal' would query either only
			//   the configured storage provider (if address is given) or all providers (or a list of providers)
			//   but with a filter by type 'personal'
			// - the question remains how would the path segments in /users be named?
			//   -> wo could use a template on the space type, to allow the admin to configure what property of a space
			//      should be used to map the initial name, eg:
			//      - {.Name} for the name of the space, makes sense for Project space
			//      - {.Owner.Username} for /users to get a list of user readable path segments ... but ... what about collisions
			//         -> append suffix?
			//      - {.Owner.Id} or {.ID} or {.Root.Id.Opaqueid} for a uuid identifier, eg for the /spaces path

			p.ProviderPath, err = rule.ProviderPath(currentUser, space)
			if err != nil {
				appctx.GetLogger(ctx).Error().Err(err).Interface("rule", rule).Interface("space", space).Msg("failed to execute template, continuing")
				continue
			}
			aliases[p.ProviderPath] = &spaceAndProvider{
				space, []*registrypb.ProviderInfo{p},
			}
		}

	}
	providers := make([]*registrypb.ProviderInfo, 0, len(aliases))
	deepestMountPath := ""
	for mountPath, spaceAndProvider := range aliases {
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
		providers = append(providers, aliases[deepestMountPath].providers...)
	}
	if len(providers) == 0 {
		return nil, errtypes.NotFound("spaces registry: storage provider not found for path reference:" + ref.String())
	}
	return providers, nil
}

func (r *registry) findStorageSpaceOnProvider(ctx context.Context, addr string, filters []*provider.ListStorageSpacesRequest_Filter) ([]*provider.StorageSpace, error) {
	c, err := pool.GetStorageProviderServiceClient(addr)
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
