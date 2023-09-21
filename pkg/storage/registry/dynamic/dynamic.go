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

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registrypb "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/registry/dynamic/rewriter"
	"github.com/cs3org/reva/pkg/storage/registry/dynamic/routingtree"
	"github.com/cs3org/reva/pkg/storage/registry/registry"
	"github.com/cs3org/reva/pkg/utils/cfg"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

func init() {
	registry.Register("dynamic", New)
}

type dynamic struct {
	c   *config
	log *zerolog.Logger
	r   map[string]string
	rt  *routingtree.RoutingTree
	ur  *rewriter.UserRewriter
}

type config struct {
	Rules      map[string]string `mapstructure:"rules"`
	Rewrites   map[string]string `mapstructure:"rewrites"`
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
	annotatedLog := log.With().Str("storageprovider", "dynamic").Logger()

	rt, err := initRoutingTree(c.DBUsername, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
	if err != nil {
		return nil, errors.Wrap(err, "error initializing routing tree")
	}

	d := &dynamic{
		c:   &c,
		log: &annotatedLog,
		r:   c.Rules,
		rt:  rt,
		ur: &rewriter.UserRewriter{
			Tpls: c.Rewrites,
		},
	}

	return d, nil
}

func initRoutingTree(dbusername, dbpassword, dbhost string, dbport int, dbname string) (*routingtree.RoutingTree, error) {
	db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", dbusername, dbpassword, dbhost, dbport, dbname))
	if err != nil {
		return nil, errors.Wrap(err, "error opening sql connection")
	}

	results, err := db.Query("SELECT path, mount_id, mount_type FROM routing")
	if err != nil {
		return nil, errors.Wrap(err, "error getting routing table from db")
	}

	var rs []routingtree.Route

	for results.Next() {
		var r routingtree.Route
		err = results.Scan(&r.Path, &r.MountID, &r.MountType)
		if err != nil {
			return nil, errors.Wrap(err, "error scanning rows from db")
		}
		rs = append(rs, r)
	}

	return routingtree.New(rs), nil
}

// ListProviders lists all available storage providers.
func (d *dynamic) ListProviders(ctx context.Context) ([]*registrypb.ProviderInfo, error) {
	providers := []*registrypb.ProviderInfo{}
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
	p, err := d.rt.GetProviders(d.c.HomePath)
	if err != nil {
		return nil, errors.New("failed to get home provider")
	}

	if a, ok := d.r[p[0]]; ok {
		return &registrypb.ProviderInfo{
			ProviderPath: d.c.HomePath,
			Address:      a,
		}, nil
	}

	return nil, errors.New("home not found")
}

// FindProviders returns the storage providers for a given ref.
func (d *dynamic) FindProviders(ctx context.Context, ref *provider.Reference) ([]*registrypb.ProviderInfo, error) {
	l := d.log.With().Str("ref", ref.String()).Logger()

	providerAlias := d.ur.GetAlias(ctx, ref.Path)
	ps, err := d.rt.GetProviders(providerAlias)
	if err != nil {
		return nil, errtypes.NotFound("storage provider not found for ref " + ref.String())
	}

	var providers []*registrypb.ProviderInfo
	for _, p := range ps {
		if address, ok := d.r[p]; ok {
			providers = append(providers, &registrypb.ProviderInfo{
				ProviderPath: ref.Path,
				Address:      address,
			})
		} else {
			err := errors.New("storage provider address not configured for mountID " + p)
			l.Error().Err(err).Msgf("error finding providers")
			return nil, errtypes.InternalError(err.Error())
		}
	}

	l.Trace().Msgf("resolved storage providers %+v", providers)

	return providers, nil
}
