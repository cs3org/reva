// Copyright 2018-2024 CERN
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

package ocdav

import (
	"context"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/rs/zerolog"
)

// logResourceAccess records who reached a resource and who owns it.
//
// The request-log middleware runs outside authentication, so it sees neither
// value; both are known here. The context logger already carries the trace id,
// which ties this to the request line.
//
// The caller goes under "accessor", not "user": other lines already log an
// object under that key, and a consumer decoding it would break on one shape
// or the other.
func logResourceAccess(ctx context.Context, log *zerolog.Logger, info *provider.ResourceInfo) {
	if info == nil || info.Owner == nil {
		return
	}
	user, ok := appctx.ContextGetUser(ctx)
	if !ok || user.Id == nil {
		return
	}
	log.Info().
		Str("accessor", user.Id.OpaqueId).
		Str("owner", info.Owner.OpaqueId).
		Bool("shared", !isCurrentUserOwner(ctx, info.Owner)).
		Msg("accessed resource")
}
