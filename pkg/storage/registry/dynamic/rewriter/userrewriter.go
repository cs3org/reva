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

// Package rewriter contains the route rewriters
package rewriter

import (
	"context"
	"errors"
	"strings"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/storage/utils/templates"
)

// UserRewriter rewrites a route with data from a user.
type UserRewriter struct {
	Tpls map[string]string
}

func (ur UserRewriter) getTemplate(route string) (string, error) {
	for key, tpl := range ur.Tpls {
		if strings.HasPrefix(route, key) {
			return tpl, nil
		}
	}

	return "", errors.New("no rewrite rule found for route")
}

// GetAlias returns the alias for a given route.
// If an alias has not been configured for the route, it returns the route.
func (ur UserRewriter) GetAlias(ctx context.Context, route string) string {
	tpl, err := ur.getTemplate(route)
	if err != nil {
		return route
	}

	if u, ok := appctx.ContextGetUser(ctx); ok {
		return templates.WithUser(u, tpl)
	}

	return route
}
