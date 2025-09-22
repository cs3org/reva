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

	"github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/preferences"
	"github.com/cs3org/reva/v3/pkg/preferences/registry"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/pkg/errors"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

type Preference struct {
	gorm.Model
	UserId      string `gorm:"size:255;index:i_user_id;uniqueIndex:i_unique"`
	Namespace   string `gorm:"size:255;index:i_namespace;uniqueIndex:i_unique"`
	ConfigKey   string `gorm:"size:255;index:i_config_key;uniqueIndex:i_unique"`
	ConfigValue string
}

// New returns an instance of the cbox sql preferences manager.
func New(ctx context.Context, m map[string]interface{}) (preferences.Manager, error) {
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
		return nil, errors.Wrap(err, "Failed to connect to preferences database")
	}

	// Migrate schemas
	err = db.AutoMigrate(&Preference{})

	if err != nil {
		return nil, errors.Wrap(err, "Failed to mgirate Preference schema")
	}

	return &mgr{
		c:  &c,
		db: db,
	}, nil
}

func (m *mgr) SetKey(ctx context.Context, key, namespace, value string) error {
	log := appctx.GetLogger(ctx)

	user, ok := appctx.ContextGetUser(ctx)
	if !ok {
		return errtypes.UserRequired("preferences: error getting user from ctx")
	}
	log.Debug().Msgf("[Preferences] Setting %s=%s in namespace %s for user %s", key, value, namespace, user.Id.OpaqueId)
	preference := &Preference{
		UserId:      user.Id.OpaqueId,
		Namespace:   namespace,
		ConfigKey:   key,
		ConfigValue: value,
	}
	res := m.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "user_id"},
			{Name: "namespace"},
			{Name: "config_key"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"config_value", "updated_at"}),
	}).Create(preference)

	return res.Error
}

func (m *mgr) GetKey(ctx context.Context, key, namespace string) (string, error) {
	log := appctx.GetLogger(ctx)

	user, ok := appctx.ContextGetUser(ctx)
	if !ok {
		return "", errtypes.UserRequired("preferences: error getting user from ctx")
	}
	query := m.db.Model(&Preference{}).
		Where("user_id = ?", user.Id.OpaqueId).
		Where("namespace = ?", namespace).
		Where("config_key = ?", key)

	fetchedPreference := &Preference{}
	res := query.First(fetchedPreference)
	log.Debug().Err(res.Error).Msgf("[Preferences] Fetched %s=%s in namespace %s for user %s", key, fetchedPreference.ConfigValue, namespace, user.Id.OpaqueId)

	if res.Error != nil {
		log.Error().Err(res.Error).Msg("Preferences GetKey: database error")
		return "", res.Error
	}

	return fetchedPreference.ConfigValue, nil
}
