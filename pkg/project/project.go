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

package project

import (
	"context"
)

type (
	// Project represents a collaborative shared space owned by an account
	// with three groups for its management.
	Project struct {
		Name         string
		Path         string
		Owner        string
		AdminGroup   string
		ReadersGroup string
		WritersGroup string
	}

	// Manager manipulates the registered projects.
	// TODO(labkode): add CRUD
	Manager interface {
		GetAllProjects(ctx context.Context) ([]*Project, error)
		GetProject(ctx context.Context, name string) (*Project, error)
	}
)
