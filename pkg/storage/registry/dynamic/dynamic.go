// Copyright 2018-2023 CERN
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

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registrypb "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/registry/dynamic/rewriter"
	"github.com/cs3org/reva/pkg/storage/registry/dynamic/routingtree"
	"github.com/cs3org/reva/pkg/storage/registry/registry"
	"github.com/cs3org/reva/pkg/storage/registry/utils"
	"github.com/cs3org/reva/pkg/utils/cfg"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

func init() {
	registry.Register("dynamic", New)
}

type dynamic struct {
	c       *config
	log     *zerolog.Logger
	aliases map[string]string
	r       map[string]string
	rt      *routingtree.RoutingTree
	ur      *rewriter.UserRewriter
}

type config struct {
	Rules      map[string]string `docs:"nil;A map from mountID to provider address"                          mapstructure:"rules"`
	Rewrites   map[string]string `docs:"nil;A map from a path to an template alias to use when resolving"    mapstructure:"rewrites"`
	IDAliases  map[string]string `docs:"nil;A map containing storageID aliases, can contain simple brackets" mapstructure:"aliases"`
	HomePath   string            `mapstructure:"home_path"`
	DBUsername string            `mapstructure:"db_username"`
	DBPassword string            `mapstructure:"db_password"`
	DBHost     string            `mapstructure:"db_host"`
	DBPort     int               `mapstructure:"db_port"`
	DBName     string            `mapstructure:"db_name"`
}

// New returns an implementation of the storage.Registry interface that
// redirects requests to corresponding storage drivers.
func New(ctx context.Context, m map[string]interface{}) (storage.Registry, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	log := appctx.GetLogger(ctx)
	annotatedLog := log.With().Str("storageregistry", "dynamic").Logger()

	rt, err := initRoutingTree(c.DBUsername, c.DBPassword, c.DBHost, c.DBPort, c.DBName, c.Rules)
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
	}

	return d, nil
}

func initRoutingTree(dbUsername, dbPassword, dbHost string, dbPort int, dbName string, rules map[string]string) (*routingtree.RoutingTree, error) {
	db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", dbUsername, dbPassword, dbHost, dbPort, dbName))
	if err != nil {
		return nil, errors.Wrap(err, "error opening sql connection")
	}

	results, err := db.Query("SELECT path, mount_id FROM routing")
	if err != nil {
		return nil, errors.Wrap(err, "error getting routing table from db")
	}

	rs := make(map[string]string)

	missingRules := []string{}

	for results.Next() {
		var p, m string
		err = results.Scan(&p, &m)
		if err != nil {
			return nil, errors.Wrap(err, "error scanning rows from db")
		}
		if _, ok := rules[m]; !ok {
			missingRules = append(missingRules, m)
		}
		rs[p] = m
	}

	if len(missingRules) != 0 {
		return nil, errors.New("config: missing routes for: " + strings.Join(missingRules, ", "))
	}

	return routingtree.New(rs), nil
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
	p, err := d.rt.Resolve(providerAlias)
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
	ps, err := d.rt.Resolve(providerAlias)
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
			err := errtypes.InternalError("storage provider address not configured for mountID " + p.ProviderId)
			l.Error().Err(err).Send()
			return nil, err
		}
	}

	l.Debug().Msgf("resolved storage providers %+v", providers)

	return providers, nil
}
