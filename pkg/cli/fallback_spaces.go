// Copyright 2018-2026 CERN
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

package cli

import (
	"context"

	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
)

// SpacesAdmin returns the client of the SpacesAdmin service on the admin
// connection. Its calls need the ctx of Admin or AdminContext, which the
// lookup and the call of an owner fallback get. UpdateSpace sets fallback
// from InFallback(ctx), for the audit.
func (r *Runtime) SpacesAdmin(ctx context.Context) (adminpb.SpacesAdminClient, error) {
	c, err := r.adminClient(ctx)
	if err != nil {
		return nil, err
	}
	return adminpb.NewSpacesAdminClient(c.Conn), nil
}
