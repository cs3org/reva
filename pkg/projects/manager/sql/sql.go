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

package sql

import (
	"context"
	"database/sql"
	"fmt"
	"slices"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/pkg/projects"
	"github.com/cs3org/reva/pkg/projects/manager/registry"
	"github.com/cs3org/reva/pkg/spaces"
	"github.com/cs3org/reva/pkg/utils/cfg"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("sql", New)
}

// Config is the configuration to use for the mysql driver
// implementing the projects.Catalogue interface.
type Config struct {
	DBUsername string `mapstructure:"db_username"`
	DBPassword string `mapstructure:"db_password"`
	DBAddress  string `mapstructure:"db_address"`
	DBName     string `mapstructure:"db_name"`
}

type mgr struct {
	c  *Config
	db *sql.DB
}

func New(ctx context.Context, m map[string]any) (projects.Catalogue, error) {
	var c Config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	return NewFromConfig(ctx, &c)
}

// Project represents a project in the DB.
type Project struct {
	StorageID string
	Path      string
	Name      string
	Owner     string
	Readers   string
	Writers   string
	Admins    string
}

// NewFromConfig creates a Repository with a SQL driver using the given config.
func NewFromConfig(ctx context.Context, conf *Config) (projects.Catalogue, error) {
	db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s)/%s", conf.DBUsername, conf.DBPassword, conf.DBAddress, conf.DBName))
	if err != nil {
		return nil, errors.Wrap(err, "sql: error opening connection to mysql database")
	}

	m := &mgr{
		c:  conf,
		db: db,
	}
	return m, nil
}

func (m *mgr) ListProjects(ctx context.Context, user *userpb.User) ([]*provider.StorageSpace, error) {
	// TODO: for the time being we load everything in memory. We may find a better
	// solution in future when the number of projects will grow.
	query := "SELECT storage_id, path, name, owner, readers, writers, admins FROM projects"
	results, err := m.db.QueryContext(ctx, query)
	if err != nil {
		return nil, errors.Wrap(err, "error getting projects from db")
	}

	var dbProjects []*Project
	for results.Next() {
		var p Project
		if err := results.Scan(&p.StorageID, &p.Path, &p.Name, &p.Owner, &p.Readers, &p.Writers, &p.Admins); err != nil {
			return nil, errors.Wrap(err, "error scanning rows from db")
		}
		dbProjects = append(dbProjects, &p)
	}

	projects := []*provider.StorageSpace{}
	for _, p := range dbProjects {
		if perms, ok := projectBelongToUser(user, p); ok {
			projects = append(projects, &provider.StorageSpace{
				Id: &provider.StorageSpaceId{
					OpaqueId: spaces.EncodeSpaceID(p.StorageID, p.Path),
				},
				Owner: &userpb.User{
					Id: &userpb.UserId{
						OpaqueId: p.Owner,
					},
				},
				Name:      p.Name,
				SpaceType: spaces.SpaceTypeProject.AsString(),
				RootInfo: &provider.ResourceInfo{
					Path:          p.Path,
					PermissionSet: perms,
				},
			})
		}
	}

	return projects, nil
}

func projectBelongToUser(user *userpb.User, p *Project) (*provider.ResourcePermissions, bool) {
	if user.Id.OpaqueId == p.Owner {
		return conversions.NewManagerRole().CS3ResourcePermissions(), true
	}
	if slices.Contains(user.Groups, p.Admins) {
		return conversions.NewManagerRole().CS3ResourcePermissions(), true
	}
	if slices.Contains(user.Groups, p.Writers) {
		return conversions.NewEditorRole().CS3ResourcePermissions(), true
	}
	if slices.Contains(user.Groups, p.Readers) {
		return conversions.NewViewerRole().CS3ResourcePermissions(), true
	}
	return nil, false
}
