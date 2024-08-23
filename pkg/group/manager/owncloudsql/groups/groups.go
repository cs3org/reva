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

package groups

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/cs3org/reva/v2/pkg/appctx"
	"github.com/pkg/errors"
)

// Groups represents oc10-style groups
type Groups struct {
	driver                       string
	db                           *sql.DB
	joinUUID, enableMedialSearch bool
	selectSQL                    string
}

// NewMysql returns a new Cache instance connecting to a MySQL database
func NewMysql(dsn string, joinUUID, enableMedialSearch bool) (*Groups, error) {
	sqldb, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, errors.Wrap(err, "error connecting to the database")
	}

	// FIXME make configurable
	sqldb.SetConnMaxLifetime(time.Minute * 3)
	sqldb.SetConnMaxIdleTime(time.Second * 30)
	sqldb.SetMaxOpenConns(100)
	sqldb.SetMaxIdleConns(10)

	err = sqldb.Ping()
	if err != nil {
		return nil, errors.Wrap(err, "error connecting to the database")
	}

	return New("mysql", sqldb, joinUUID, enableMedialSearch)
}

// New returns a new Cache instance connecting to the given sql.DB
func New(driver string, sqldb *sql.DB, joinUUID, enableMedialSearch bool) (*Groups, error) {

	sel := "SELECT gid"
	from := `
		FROM oc_groups g
		`

	return &Groups{
		driver:             driver,
		db:                 sqldb,
		joinUUID:           joinUUID,
		enableMedialSearch: enableMedialSearch,
		selectSQL:          sel + from,
	}, nil
}

// Group stores information about groups, which ... in oc10 is not much
//
//	DESCRIBE oc_groups;
//	+-------+-------------+------+-----+---------+-------+
//	| Field | Type        | Null | Key | Default | Extra |
//	+-------+-------------+------+-----+---------+-------+
//	| gid   | varchar(64) | NO   | PRI |         |       |
//	+-------+-------------+------+-----+---------+-------+
type Group struct {
	GID string
}

func (as *Groups) rowToGroup(ctx context.Context, row Scannable) (*Group, error) {
	g := Group{}
	if err := row.Scan(&g.GID); err != nil {
		appctx.GetLogger(ctx).Error().Err(err).Msg("could not scan row, skipping")
		return nil, err
	}

	return &g, nil
}

// Scannable describes the interface providing a Scan method
type Scannable interface {
	Scan(...interface{}) error
}

// GetGroupByClaim fetches a group by group_name or group_id
func (gs *Groups) GetGroupByClaim(ctx context.Context, claim, value string) (*Group, error) {
	// TODO align supported claims with rest driver and the others, maybe refactor into common mapping
	var row *sql.Row
	var where string
	switch claim {
	//case "mail":
	//	where = "WHERE a.email=?"
	// case "uid":
	//	claim = m.c.Schema.UIDNumber
	// case "gid":
	//	claim = m.c.Schema.GIDNumber
	case "display_name":
		// use gid as username
		where = "WHERE g.gid=?"
	case "group_name":
		// use gid as username
		where = "WHERE g.gid=?"
	case "group_id":
		// use gid as uuid
		where = "WHERE g.gid=?"
	default:
		return nil, errors.New("owncloudsql: invalid field " + claim)
	}

	row = gs.db.QueryRowContext(ctx, gs.selectSQL+where, value)

	return gs.rowToGroup(ctx, row)
}

func sanitizeWildcards(q string) string {
	return strings.ReplaceAll(strings.ReplaceAll(q, "%", `\%`), "_", `\_`)
}

// FindGroups searches gid using the given query. The Wildcard caracters % and _ are escaped.
func (gs *Groups) FindGroups(ctx context.Context, query string) ([]Group, error) {
	if gs.enableMedialSearch {
		query = "%" + sanitizeWildcards(query) + "%"
	}

	where := "WHERE g.gid LIKE ?"
	args := []interface{}{query, query, query}

	rows, err := gs.db.QueryContext(ctx, gs.selectSQL+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groups := []Group{}
	for rows.Next() {
		g := Group{}
		if err := rows.Scan(&g.GID); err != nil {
			appctx.GetLogger(ctx).Error().Err(err).Msg("could not scan row, skipping")
			continue
		}
		groups = append(groups, g)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return groups, nil
}

// GetMembers lists the members for a group
func (gs *Groups) GetMembers(ctx context.Context, gid string) ([]string, error) {
	rows, err := gs.db.QueryContext(ctx, "SELECT uid FROM oc_group_user WHERE gid=?", gid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	members := []string{}
	for rows.Next() {
		var member string
		if err := rows.Scan(&member); err != nil {
			appctx.GetLogger(ctx).Error().Err(err).Msg("could not scan row, skipping")
			continue
		}
		members = append(members, member)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return members, nil
}

// HasMember lists the members for a group
func (gs *Groups) HasMember(ctx context.Context, gid, uid string) (bool, error) {
	row, err := gs.db.QueryContext(ctx, "SELECT count(*) as count FROM oc_group_user WHERE gid=? AND uid=?", gid, uid)
	if err != nil {
		return false, err
	}
	defer row.Close()

	var count int
	err = row.Scan(&count)

	return count > 0, err
}
