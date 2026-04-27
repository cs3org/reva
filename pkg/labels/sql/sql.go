// Copyright 2018-2026 CERN
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
	"time"

	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/pkg/errors"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/labels/registry"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
)

func init() {
	registry.Register("sql", New)
}

type Config struct {
	config.Database `mapstructure:",squash"`
}

func (c *Config) ApplyDefaults() {
	c.Database = sharedconf.GetDBInfo(c.Database)
}

type mgr struct {
	c  *Config
	db *gorm.DB
}

type Label struct {
	// We don't use gorm.Model since we want to add an index on DeletedAt
	//gorm.Model
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"uniqueIndex:u_label;index"`

	Inode    string `gorm:"size:32;uniqueIndex:u_label;index"`
	Instance string `gorm:"size:32;uniqueIndex:u_label;index"`
	UserId   string `gorm:"size:64;uniqueIndex:u_label;index"`
	Label    string `gorm:"size:64;uniqueIndex:u_label;index"`
}

// New returns an instance of the cbox sql labels manager.
func New(m map[string]any) (labels.Manager, error) {
	var c Config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	c.ApplyDefaults()

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
		return nil, errors.Wrap(err, "Failed to connect to favorites database using engine "+c.Engine)
	}

	// Migrate schemas
	err = db.AutoMigrate(&Label{})

	if err != nil {
		return nil, errors.Wrap(err, "Failed to mgirate favorites schema")
	}

	return &mgr{
		c:  &c,
		db: db,
	}, nil
}

func (m *mgr) ListFavorites(ctx context.Context, label string, userID *user.UserId) ([]*provider.ResourceId, error) {
	log := appctx.GetLogger(ctx)

	user, ok := appctx.ContextGetUser(ctx)
	if !ok {
		return nil, errtypes.UserRequired("favorites: error getting user from ctx")
	}

	query := m.db.Model(&Label{}).
		Where("user_id = ?", user.Id.OpaqueId).
		Where("label = ?", label)

	fetchedResults := []Label{}
	res := query.First(&fetchedResults)

	if res.Error != nil {
		log.Error().Err(res.Error).Msg("ListFavorites: database error")
		return nil, res.Error
	}

	infos := []*provider.ResourceId{}
	for _, label := range fetchedResults {
		infos = append(infos, &provider.ResourceId{
			StorageId: label.Instance,
			OpaqueId:  label.Inode,
		})
	}

	return infos, nil
}

func (m *mgr) SetLabel(ctx context.Context, label string, resourceInfo *provider.ResourceInfo) error {
	log := appctx.GetLogger(ctx)

	user, ok := appctx.ContextGetUser(ctx)
	if !ok {
		return errtypes.UserRequired("favorites: error getting user from ctx")
	}

	l := &Label{
		UserId:   user.Id.OpaqueId,
		Inode:    resourceInfo.Id.OpaqueId,
		Instance: resourceInfo.Id.StorageId,
	}
	res := m.db.Create(l)

	log.Debug().Err(res.Error).Msgf("Set label for %+v", l)

	return res.Error
}

func (m *mgr) UnsetLabel(ctx context.Context, label string, resourceInfo *provider.ResourceInfo) error {
	log := appctx.GetLogger(ctx)

	user, ok := appctx.ContextGetUser(ctx)
	if !ok {
		return errtypes.UserRequired("favorites: error getting user from ctx")
	}

	query := m.db.
		Where("user_id = ?", user.Id.OpaqueId).
		Where("inode = ?", resourceInfo.Id.OpaqueId).
		Where("instance = ?", resourceInfo.Id.StorageId).
		Where("label = ?", label)

	res := query.Delete(&Label{})

	log.Debug().Err(res.Error).Msgf("Delete label %s for (%s, %s) for user %s", label, resourceInfo.Id.OpaqueId, resourceInfo.Id.StorageId, user.Id.OpaqueId)

	return res.Error
}
