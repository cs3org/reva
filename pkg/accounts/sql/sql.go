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

	"github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/pkg/errors"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Config is the configuration to use for the sql driver
// connecting to the accounts database.
type Config struct {
	config.Database `mapstructure:",squash"`
}

func (c *Config) ApplyDefaults() {
	c.Database = sharedconf.GetDBInfo(c.Database)
}

// Account represents a row in the `account` table.
type Account struct {
	GUID               string    `gorm:"column:guid;type:varchar(36);primaryKey"`
	UniqueIdentifier   string    `gorm:"column:unique_identifier;type:varchar(255);not null;uniqueIndex:i_unique_identifier"`
	SubscriptionStatus string    `gorm:"column:subscription_status;type:varchar(50)"`
	AccountType        string    `gorm:"column:account_type;type:varchar(50)"`
	OwnerID            string    `gorm:"column:owner_id;type:varchar(36)"`
	CreatedAt          time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt          time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

// TableName pins the table name to `account`, matching the existing schema
// instead of gorm's default pluralized `accounts`.
func (Account) TableName() string {
	return "account"
}

// New opens a connection to the accounts database and migrates the Account
// schema into it.
func New(ctx context.Context, m map[string]any) (*gorm.DB, error) {
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
		return nil, errors.Wrap(err, "Failed to connect to Accounts database using engine "+c.Engine)
	}

	if err := db.AutoMigrate(&Account{}); err != nil {
		return nil, errors.Wrap(err, "Failed to migrate Account schema")
	}

	return db, nil
}
