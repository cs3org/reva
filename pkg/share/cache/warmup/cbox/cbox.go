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

package cbox

import (
	"context"
	"database/sql"
	"fmt"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/auth/scope"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/share/cache"
	"github.com/cs3org/reva/pkg/share/cache/warmup/registry"
	"github.com/cs3org/reva/pkg/token/manager/jwt"

	// Provides mysql drivers.
	_ "github.com/go-sql-driver/mysql"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/grpc/metadata"
)

func init() {
	registry.Register("cbox", New)
}

type config struct {
	DBUsername   string `mapstructure:"db_username"`
	DBPassword   string `mapstructure:"db_password"`
	DBHost       string `mapstructure:"db_host"`
	DBPort       int    `mapstructure:"db_port"`
	DBName       string `mapstructure:"db_name"`
	EOSNamespace string `mapstructure:"namespace"`
	GatewaySvc   string `mapstructure:"gatewaysvc"`
	JWTSecret    string `mapstructure:"jwt_secret"`
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

// New returns an implementation of cache warmup that connects to the cbox share db and stats resources on EOS.
func New(m map[string]interface{}) (cache.Warmup, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", c.DBUsername, c.DBPassword, c.DBHost, c.DBPort, c.DBName))
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

	tokenManager, err := jwt.New(map[string]interface{}{
		"secret": m.conf.JWTSecret,
	})
	if err != nil {
		return nil, err
	}

	u := &userpb.User{
		Id: &userpb.UserId{
			OpaqueId: "root",
		},
		UidNumber: 0,
		GidNumber: 0,
	}
	scope, err := scope.AddOwnerScope(nil)
	if err != nil {
		return nil, err
	}

	tkn, err := tokenManager.MintToken(context.Background(), u, scope)
	if err != nil {
		return nil, err
	}
	ctx := metadata.AppendToOutgoingContext(context.Background(), ctxpkg.TokenHeader, tkn)

	client, err := pool.GetGatewayServiceClient(pool.Endpoint(m.conf.GatewaySvc))
	if err != nil {
		return nil, err
	}

	infos := []*provider.ResourceInfo{}
	for rows.Next() {
		var storageID, nodeID string
		if err := rows.Scan(&storageID, &nodeID); err != nil {
			continue
		}

		statReq := provider.StatRequest{Ref: &provider.Reference{
			ResourceId: &provider.ResourceId{
				StorageId: storageID,
				OpaqueId:  nodeID,
			},
		}}

		statRes, err := client.Stat(ctx, &statReq)
		if err != nil || statRes.Status.Code != rpc.Code_CODE_OK {
			continue
		}

		infos = append(infos, statRes.Info)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return infos, nil
}
