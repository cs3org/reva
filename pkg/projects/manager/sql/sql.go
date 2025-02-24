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
	"fmt"
	"slices"
	"time"

	"github.com/ReneKroon/ttlcache/v2"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/pkg/projects"
	"github.com/cs3org/reva/pkg/projects/manager/registry"
	"github.com/cs3org/reva/pkg/spaces"
	"github.com/cs3org/reva/pkg/utils/cfg"
	"github.com/pkg/errors"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func init() {
	registry.Register("sql", New)
}

// Config is the configuration to use for the mysql driver
// implementing the projects.Catalogue interface.
type Config struct {
	Engine     string `mapstructure:"engine"` // mysql | sqlite
	DBUsername string `mapstructure:"db_username"`
	DBPassword string `mapstructure:"db_password"`
	DBHost     string `mapstructure:"db_host"`
	DBPort     int    `mapstructure:"db_port"`
	DBName     string `mapstructure:"db_name"`
	// CacheTTL (seconds) determines how long the list of projects will be stored in a cache
	// before a new database query is executed. The default, 0, corresponds to 60 seconds.
	CacheTTL int `mapstructure:"cache_ttl"`
}

type mgr struct {
	c     *Config
	db    *gorm.DB
	cache *ttlcache.Cache
}

const cacheKey = "projects/projectsListCache"

// Project represents a project in the DB.
type Project struct {
	gorm.Model
	StorageID string `gorm:"size:255"`
	Path      string
	Name      string `gorm:"size:255;uniqueIndex:i_name"`
	Owner     string `gorm:"size:255"`
	Readers   string
	Writers   string
	Admins    string
}

func New(ctx context.Context, m map[string]any) (projects.Catalogue, error) {
	var c Config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	var db *gorm.DB
	var err error
	switch c.Engine {
	case "sqlite":
		db, err = gorm.Open(sqlite.Open(c.DBName), &gorm.Config{})
	case "mysql":
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true", c.DBUsername, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	default: // default is mysql
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true", c.DBUsername, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	}
	if err != nil {
		return nil, errors.Wrap(err, "Failed to connect to Projects database")
	}

	// Migrate schemas
	err = db.AutoMigrate(&Project{})

	if err != nil {
		return nil, errors.Wrap(err, "Failed to mgirate Project schema")
	}

	cache := ttlcache.NewCache()
	if c.CacheTTL == 0 {
		c.CacheTTL = 60
	}
	cache.SetTTL(time.Duration(c.CacheTTL))
	// Even if we get a hit, of course we just want to refresh every 60 seconds
	cache.SkipTTLExtensionOnHit(true)
	mgr := &mgr{
		c:     &c,
		db:    db,
		cache: cache,
	}
	return mgr, nil
}

func (m *mgr) ListProjects(ctx context.Context, user *userpb.User) ([]*provider.StorageSpace, error) {
	var fetchedProjects []*Project

	if res, err := m.cache.Get(cacheKey); err == nil && res != nil {
		fetchedProjects = res.([]*Project)
	} else {
		query := m.db.Model(&Project{})
		res := query.Find(&fetchedProjects)
		if res.Error != nil {
			return nil, res.Error
		}
		m.cache.Set(cacheKey, fetchedProjects)
	}

	projects := []*provider.StorageSpace{}
	for _, p := range fetchedProjects {
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
