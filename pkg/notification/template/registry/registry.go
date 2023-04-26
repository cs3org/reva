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

package registry

import (
	"encoding/json"
	"fmt"

	"github.com/cs3org/reva/pkg/notification/handler"
	"github.com/cs3org/reva/pkg/notification/template"
	"github.com/pkg/errors"
)

// Registry provides with means for dynamically registering notification templates.
type Registry struct {
	store map[string]template.Template
}

// New returns a new Template Registry.
func New() *Registry {
	r := &Registry{
		store: make(map[string]template.Template),
	}

	return r
}

// Put registers a handler in the registry.
func (r *Registry) Put(tb []byte, hs map[string]handler.Handler) (string, error) {
	var data map[string]interface{}

	err := json.Unmarshal(tb, &data)
	if err != nil {
		return "", errors.Wrapf(err, "template registration unmarshall failed")
	}

	t, name, err := template.New(data, hs)
	if err != nil {
		return name, errors.Wrapf(err, "template %s registration failed", name)
	}

	r.store[t.Name] = *t
	return t.Name, nil
}

// Get retrieves a handler from the registry.
func (r *Registry) Get(n string) (*template.Template, error) {
	if t, ok := r.store[n]; ok {
		return &t, nil
	}

	return nil, fmt.Errorf("template %s not found", n)
}
