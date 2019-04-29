// Copyright 2018-2019 CERN
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

package projdb

import (
	"context"
	"database/sql"

	"github.com/cernbox/reva/pkg/project"
	_ "github.com/go-sql-driver/mysql" // import mysql driver
	"github.com/pkg/errors"
)

type manager struct {
	db                                     *sql.DB
	dbUsername, dbPassword, dbHost, dbName string
	dbPort                                 int
}

// New returns a new project manager that stores the project information in a mysql database.
func New(dbUsername, dbPassword, dbHost string, dbPort int, dbName string) project.Manager {
	return &manager{dbUsername: dbUsername, dbPassword: dbPassword, dbHost: dbHost, dbName: dbName, dbPort: dbPort}
}

func (m *manager) GetProject(ctx context.Context, projectName string) (*project.Project, error) {
	var (
		owner string
		path  string
	)

	query := "select eos_relative_path, project_owner from cernbox_project_mapping where project_name=?"
	if err := m.db.QueryRow(query, projectName).Scan(&path, &owner); err != nil {
		if err == sql.ErrNoRows {
			err := projectNotFoundError(projectName)
			return nil, errors.Wrapf(err, "projdb: projectName=%s not found", projectName)
		}
		return nil, errors.Wrapf(err, "projdb: error querying db for projectName=%s", projectName)
	}

	adminGroup := getAdminGroup(projectName)
	writersGroup := getWritersGroup(projectName)
	readersGroup := getReadersGroup(projectName)

	project := &project.Project{Name: projectName,
		Owner:        owner,
		Path:         path,
		AdminGroup:   adminGroup,
		ReadersGroup: readersGroup,
		WritersGroup: writersGroup}

	return project, nil

}

func getAdminGroup(name string) string   { return "cernbox-project-" + name + "-admins" }
func getReadersGroup(name string) string { return "cernbox-project-" + name + "-readers" }
func getWritersGroup(name string) string { return "cernbox-project-" + name + "-writers" }

func (m *manager) GetAllProjects(ctx context.Context) ([]*project.Project, error) {
	query := "select project_name, project_owner, eos_relative_path from cernbox_project_mapping"
	rows, err := m.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var (
		name  string
		owner string
		path  string
	)

	projects := []*project.Project{}
	for rows.Next() {
		err := rows.Scan(&name, &owner, &path)
		if err != nil {
			return nil, err
		}

		adminGroup := getAdminGroup(name)
		writersGroup := getWritersGroup(name)
		readersGroup := getReadersGroup(name)

		project := &project.Project{Owner: owner, Path: path, Name: name, AdminGroup: adminGroup, ReadersGroup: readersGroup, WritersGroup: writersGroup}
		projects = append(projects, project)

	}

	err = rows.Err()
	if err != nil {
		return nil, err
	}

	return projects, nil
}

type projectNotFoundError string

func (e projectNotFoundError) Error() string { return string(e) }
