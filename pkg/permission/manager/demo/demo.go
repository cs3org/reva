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

package demo

import (
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/permission"
	"github.com/cs3org/reva/pkg/permission/manager/registry"
)

func init() {
	registry.Register("demo", New)
}

// New returns a new demo permission manager.
func New(c map[string]interface{}) (permission.Manager, error) {
	return manager{}, nil
}

type manager struct {
}

func (m manager) CheckPermission(permission string, subject string, ref *provider.Reference) bool {
	// We can currently return true all the time.
	// Once we beginn testing roles we need to somehow check the roles of the users here
	return true
}
