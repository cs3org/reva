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

package eos

import (
	"context"
	"database/sql"
	"fmt"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/share/cache"
	"github.com/cs3org/reva/pkg/share/cache/registry"
	"github.com/cs3org/reva/pkg/storage/fs/eos"
	"github.com/cs3org/reva/pkg/user"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"

	// Provides mysql drivers
	_ "github.com/go-sql-driver/mysql"
)

func init() {
	registry.Register("cbox", New)
}

type config struct {
	DbUsername   string `mapstructure:"db_username"`
	DbPassword   string `mapstructure:"db_password"`
	DbHost       string `mapstructure:"db_host"`
	DbPort       int    `mapstructure:"db_port"`
	DbName       string `mapstructure:"db_name"`
	EOSNamespace string `mapstructure:"namespace"`
	GatewaySvc   string `mapstructure:"gatewaysvc"`
}

type manager struct {
	conf *config
	db   *sql.DB
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

// New returns a new implementation of the storage.FS interface that connects to EOS.
func New(m map[string]interface{}) (cache.Warmup, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", c.DbUsername, c.DbPassword, c.DbHost, c.DbPort, c.DbName))
	if err != nil {
		return nil, err
	}

	return &manager{
		conf: c,
		db:   db,
	}, nil
}

func (m *manager) GetResourceInfos() ([]*provider.ResourceInfo, error) {
	query := "select coalesce(fileid_prefix, '') as fileid_prefix, coalesce(item_source, '') as item_source FROM oc_share WHERE (orphan = 0 or orphan IS NULL)"
	rows, err := m.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	infos := []*provider.ResourceInfo{}
	for rows.Next() {
		var storageID, opaqueID string
		if err := rows.Scan(&storageID, &opaqueID); err != nil {
			continue
		}

		eosOpts := map[string]interface{}{
			"namespace":         m.conf.EOSNamespace,
			"master_url":        fmt.Sprintf("root://%s.cern.ch", storageID),
			"version_invariant": true,
			"gatewaysvc":        m.conf.GatewaySvc,
		}
		eos, err := eos.New(eosOpts)
		if err != nil {
			return nil, err
		}

		ctx := user.ContextSetUser(context.Background(), &userpb.User{
			Id: &userpb.UserId{
				OpaqueId: "root",
			},
			Opaque: &types.Opaque{
				Map: map[string]*types.OpaqueEntry{
					"uid": &types.OpaqueEntry{
						Decoder: "plain",
						Value:   []byte("0"),
					},
					"gid": &types.OpaqueEntry{
						Decoder: "plain",
						Value:   []byte("0"),
					},
				},
			},
		})

		inf, err := eos.GetMD(ctx, &provider.Reference{
			Spec: &provider.Reference_Id{
				Id: &provider.ResourceId{
					StorageId: storageID,
					OpaqueId:  opaqueID,
				},
			},
		}, []string{})
		if err != nil {
			return nil, err
		}
		infos = append(infos, inf)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return infos, nil

}
