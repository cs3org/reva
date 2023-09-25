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

package sql

import (
	"context"
	"database/sql"
	"fmt"

	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/preferences"
	"github.com/cs3org/reva/pkg/preferences/registry"
	"github.com/cs3org/reva/pkg/utils/cfg"
)

func init() {
	registry.Register("sql", New)
}

type config struct {
	DBUsername string `mapstructure:"db_username"`
	DBPassword string `mapstructure:"db_password"`
	DBHost     string `mapstructure:"db_host"`
	DBPort     int    `mapstructure:"db_port"`
	DBName     string `mapstructure:"db_name"`
}

type mgr struct {
	c  *config
	db *sql.DB
}

// New returns an instance of the cbox sql preferences manager.
func New(ctx context.Context, m map[string]interface{}) (preferences.Manager, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", c.DBUsername, c.DBPassword, c.DBHost, c.DBPort, c.DBName))
	if err != nil {
		return nil, err
	}

	return &mgr{
		c:  &c,
		db: db,
	}, nil
}

func (m *mgr) SetKey(ctx context.Context, key, namespace, value string) error {
	user, ok := ctxpkg.ContextGetUser(ctx)
	if !ok {
		return errtypes.UserRequired("preferences: error getting user from ctx")
	}
	query := `INSERT INTO oc_preferences(userid, appid, configkey, configvalue) values(?, ?, ?, ?) ON DUPLICATE KEY UPDATE configvalue = ?`
	params := []interface{}{user.Id.OpaqueId, namespace, key, value, value}
	stmt, err := m.db.Prepare(query)
	if err != nil {
		return err
	}

	if _, err = stmt.Exec(params...); err != nil {
		return err
	}
	return nil
}

func (m *mgr) GetKey(ctx context.Context, key, namespace string) (string, error) {
	user, ok := ctxpkg.ContextGetUser(ctx)
	if !ok {
		return "", errtypes.UserRequired("preferences: error getting user from ctx")
	}
	query := `SELECT configvalue FROM oc_preferences WHERE userid=? AND appid=? AND configkey=?`
	var val string
	if err := m.db.QueryRow(query, user.Id.OpaqueId, namespace, key).Scan(&val); err != nil {
		if err == sql.ErrNoRows {
			return "", errtypes.NotFound(namespace + ":" + key)
		}
		return "", err
	}
	return val, nil
}
