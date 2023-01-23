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

package grpc_test

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	conversions "github.com/cs3org/reva/pkg/cbox/utils"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	"github.com/cs3org/reva/tests/helpers"

	_ "github.com/go-sql-driver/mysql"
)

func initData(driver string, tokens []*invitepb.InviteToken, acceptedUsers map[string][]*userpb.User) (map[string]string, func(), error) {
	variables := map[string]string{
		"ocm_driver": driver,
	}
	switch driver {
	case "json":
		return initJSONData(variables, tokens, acceptedUsers)
	case "sql":
		return initSQLData(variables, tokens, acceptedUsers)
	}

	return nil, nil, errors.New("driver not found")
}

func initJSONData(variables map[string]string, tokens []*invitepb.InviteToken, acceptedUsers map[string][]*userpb.User) (map[string]string, func(), error) {
	data := map[string]any{}

	if len(tokens) != 0 {
		m := map[string]*invitepb.InviteToken{}
		for _, tkn := range tokens {
			m[tkn.Token] = tkn
		}
		data["invites"] = m
	}

	if len(acceptedUsers) != 0 {
		data["accepted_users"] = acceptedUsers
	}

	inviteTokenFile, err := helpers.TempJSONFile(data)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		Expect(os.RemoveAll(inviteTokenFile)).To(Succeed())
	}
	variables["invite_token_file"] = inviteTokenFile
	return variables, cleanup, nil
}

func initTables(db *sql.DB) error {
	table1 := `
CREATE TABLE IF NOT EXISTS ocm_tokens (
    token VARCHAR(255) NOT NULL PRIMARY KEY,
    initiator VARCHAR(255) NOT NULL,
    expiration DATETIME NOT NULL,
    description VARCHAR(255) DEFAULT NULL
)`
	table2 := `
CREATE TABLE IF NOT EXISTS ocm_remote_users (
    initiator VARCHAR(255) NOT NULL,
    opaque_user_id VARCHAR(255) NOT NULL,
    idp VARCHAR(255) NOT NULL,
    email VARCHAR(255) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    PRIMARY KEY (initiator, opaque_user_id, idp)
)`
	if _, err := db.Exec(table1); err != nil {
		return err
	}
	if _, err := db.Exec(table2); err != nil {
		return err
	}
	return nil
}

func dropTables(db *sql.DB) error {
	drop1 := "DROP TABLE IF EXISTS ocm_tokens"
	drop2 := "DROP TABLE IF EXISTS ocm_remote_users"
	if _, err := db.Exec(drop1); err != nil {
		return err
	}
	if _, err := db.Exec(drop2); err != nil {
		return err
	}
	return nil
}

func initSQLData(variables map[string]string, tokens []*invitepb.InviteToken, acceptedUsers map[string][]*userpb.User) (map[string]string, func(), error) {
	username := os.Getenv("SQL_USERNAME")
	password := os.Getenv("SQL_PASSWORD")
	address := os.Getenv("SQL_ADDRESS")
	database := os.Getenv("SQL_DBNAME")

	db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s)/%s", username, password, address, database))
	if err != nil {
		return nil, nil, err
	}
	if err := initTables(db); err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		Expect(dropTables(db)).To(Succeed())
	}

	variables["db_username"] = username
	variables["db_password"] = password
	variables["db_address"] = address
	variables["db_name"] = database

	if err := initTokens(db, tokens); err != nil {
		return nil, nil, err
	}
	if err := initAcceptedUsers(db, acceptedUsers); err != nil {
		return nil, nil, err
	}

	return variables, cleanup, nil
}

func initTokens(db *sql.DB, tokens []*invitepb.InviteToken) error {
	query := "INSERT INTO ocm_tokens (token, initiator, expiration, description) VALUES (?,?,?,?)"
	for _, token := range tokens {
		if _, err := db.Exec(query, token.Token, conversions.FormatUserID(token.UserId), time.Unix(int64(token.Expiration.Seconds), 0), token.Description); err != nil {
			return err
		}
	}
	return nil
}

func initAcceptedUsers(db *sql.DB, acceptedUsers map[string][]*userpb.User) error {
	query := "INSERT INTO ocm_remote_users (initiator, opaque_user_id, idp, email, display_name) VALUES (?,?,?,?,?)"
	for initiator, users := range acceptedUsers {
		for _, user := range users {
			if _, err := db.Exec(query, initiator, user.Id.OpaqueId, user.Id.Idp, user.Mail, user.DisplayName); err != nil {
				return err
			}
		}
	}
	return nil
}
