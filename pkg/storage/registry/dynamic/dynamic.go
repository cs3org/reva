// Copyright 2018-2024 CERN
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

// Package dynamic contains the dynamic storage registry
package dynamic

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registrypb "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	"github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/storage"
	"github.com/cs3org/reva/v3/pkg/storage/registry/dynamic/rewriter"
	"github.com/cs3org/reva/v3/pkg/storage/registry/dynamic/routingtree"
	"github.com/cs3org/reva/v3/pkg/storage/registry/registry"
	"github.com/cs3org/reva/v3/pkg/storage/registry/utils"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

func init() {
	registry.Register("dynamic", New)
}

type dynamic struct {
	c       *Config
	log     *zerolog.Logger
	aliases map[string]string
	r       map[string]string
	ur      *rewriter.UserRewriter
	// db is the routing table; rt is its in-memory tree, rebuilt by Reload
	// and guarded by mu.
	db *sql.DB
	mu sync.RWMutex
	rt *routingtree.RoutingTree
}

// openDB opens the routing database; a variable so tests can substitute it.
var openDB = func(dsn string) (*sql.DB, error) {
	return sql.Open("mysql", dsn)
}

type Config struct {
	Rules           map[string]string `docs:"nil;A map from mountID to provider address"                          mapstructure:"rules"`
	Rewrites        map[string]string `docs:"nil;A map from a path to an template alias to use when resolving"    mapstructure:"rewrites"`
	IDAliases       map[string]string `docs:"nil;A map containing storageID aliases, can contain simple brackets" mapstructure:"aliases"`
	HomePath        string            `mapstructure:"home_path"`
	config.Database `mapstructure:",squash"`
}

func (c *Config) ApplyDefaults() {
	c.Database = sharedconf.GetDBInfo(c.Database)
}

// New returns an implementation of the storage.Registry interface that
// redirects requests to corresponding storage drivers.
func New(ctx context.Context, m map[string]any) (storage.Registry, error) {
	var c Config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	c.ApplyDefaults()

	log := appctx.GetLogger(ctx)
	annotatedLog := log.With().Str("storageregistry", "dynamic").Logger()

	db, err := openDB(fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", c.DBUsername, c.DBPassword, c.DBHost, c.DBPort, c.DBName))
	if err != nil {
		return nil, errors.Wrap(err, "error opening sql connection")
	}
	rt, err := loadRoutingTree(ctx, db, c.Rules)
	if err != nil {
		return nil, errors.Wrap(err, "error initializing routing tree")
	}

	aliases := initAliases(c.IDAliases)

	d := &dynamic{
		c:       &c,
		log:     &annotatedLog,
		r:       c.Rules,
		aliases: aliases,
		rt:      rt,
		ur: &rewriter.UserRewriter{
			Tpls: c.Rewrites,
		},
		db: db,
	}

	return d, nil
}

// loadRoutingTree reads the routing table and builds the tree of the routes
// whose mount is configured in rules; the others are logged and skipped.
func loadRoutingTree(ctx context.Context, db *sql.DB, rules map[string]string) (*routingtree.RoutingTree, error) {
	log := appctx.GetLogger(ctx).With().Str("storageregistry", "dynamic").Logger()

	routes, err := readRoutes(ctx, db)
	if err != nil {
		return nil, err
	}

	rs := make(map[string]string, len(routes))
	missingRules := []string{}
	for _, r := range routes {
		if _, ok := rules[r.MountID]; !ok {
			missingRules = append(missingRules, r.Path+" => "+r.MountID)
		} else {
			rs[r.Path] = r.MountID
		}
	}

	if len(missingRules) != 0 {
		log.Error().Msgf("routes without a configured mount, skipped: %s", strings.Join(missingRules, ", "))
	}

	return routingtree.New(rs), nil
}

// readRoutes reads the routing table, by path.
func readRoutes(ctx context.Context, db *sql.DB) ([]Route, error) {
	results, err := db.QueryContext(ctx, "SELECT path, mount_id FROM routing ORDER BY path")
	if err != nil {
		return nil, errors.Wrap(err, "error getting routing table from db")
	}
	defer results.Close()

	var routes []Route
	for results.Next() {
		var r Route
		if err := results.Scan(&r.Path, &r.MountID); err != nil {
			return nil, errors.Wrap(err, "error scanning rows from db")
		}
		routes = append(routes, r)
	}
	return routes, results.Err()
}

// tree is the current routing tree.
func (d *dynamic) tree() *routingtree.RoutingTree {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.rt
}

func initAliases(aliasesConfig map[string]string) map[string]string {
	aliases := make(map[string]string)

	for aliasRegex, storageID := range aliasesConfig {
		expandedAliases := utils.GenerateRegexCombinations(aliasRegex)
		for _, alias := range expandedAliases {
			aliases[alias] = storageID
		}
	}

	return aliases
}

// ListProviders lists all available storage providers.
func (d *dynamic) ListProviders(ctx context.Context) ([]*registrypb.ProviderInfo, error) {
	providers := make([]*registrypb.ProviderInfo, 0, len(d.r))
	for p, a := range d.r {
		providers = append(providers, &registrypb.ProviderInfo{
			ProviderPath: p,
			Address:      a,
		})
	}

	return providers, nil
}

// GetHome returns the storage provider for the home path.
func (d *dynamic) GetHome(ctx context.Context) (*registrypb.ProviderInfo, error) {
	providerAlias := d.ur.GetAlias(ctx, d.c.HomePath)
	p, err := d.tree().Resolve(providerAlias)
	if err != nil {
		return nil, errors.New("failed to get home provider")
	}

	if a, ok := d.r[p[0].ProviderId]; ok {
		return &registrypb.ProviderInfo{
			ProviderPath: d.c.HomePath,
			Address:      a,
		}, nil
	}

	return nil, errtypes.InternalError("storage provider address not configured for mountID " + p[0].ProviderId)
}

// FindProviders returns the storage providers for a given ref.
func (d *dynamic) FindProviders(ctx context.Context, ref *provider.Reference) ([]*registrypb.ProviderInfo, error) {
	l := d.log.With().Interface("ref", ref).Logger()

	l.Trace().Msg("Finding providers")

	if ref.ResourceId != nil && ref.ResourceId.StorageId != "" {
		storageID := ref.ResourceId.StorageId

		// Resolve aliased storageIDs (like eoshome-a into eoshome-i01)
		if i, ok := d.aliases[storageID]; ok {
			storageID = i
		}

		if address, ok := d.r[storageID]; ok {
			return []*registrypb.ProviderInfo{{
				ProviderId: ref.ResourceId.StorageId,
				Address:    address,
			}}, nil
		}

		err := errtypes.InternalError("storage provider address not found for " + storageID)
		l.Error().Err(err).Send()
		return nil, err
	}

	providerAlias := d.ur.GetAlias(ctx, ref.Path)
	ps, err := d.tree().Resolve(providerAlias)
	if err != nil {
		return nil, errtypes.NotFound("storage provider not found for ref " + ref.String())
	}

	var providers []*registrypb.ProviderInfo
	for _, p := range ps {
		if address, ok := d.r[p.ProviderId]; ok {
			providers = append(providers, &registrypb.ProviderInfo{
				ProviderPath: p.ProviderPath,
				Address:      address,
			})
		} else {
			l.Error().Str("mount", p.ProviderId).Msg("Storage provider address not configured for mount, skipping")
		}
	}

	l.Debug().Msgf("resolved storage providers %+v for the given ref", providers)
	return providers, nil
}
