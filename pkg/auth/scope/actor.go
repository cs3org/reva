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

package scope

import (
	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
)

// ActorScope carries who impersonates the user of a token. It has no verifier,
// so it authorizes nothing. It is a scope and not a JWT claim because the auth
// cache and the token re-mints keep only the user and the scopes.
const ActorScope = "actor"

// AddActorScope records actor as the impersonator: a user id, or
// "socket:<uid>" for a caller on the local-root socket.
func AddActorScope(actor string, scopes map[string]*authpb.Scope) map[string]*authpb.Scope {
	if scopes == nil {
		scopes = make(map[string]*authpb.Scope)
	}
	scopes[ActorScope] = &authpb.Scope{
		Resource: &types.OpaqueEntry{Decoder: "plain", Value: []byte(actor)},
	}
	return scopes
}

// Actor returns the impersonator recorded in scopes, or "" for a token that
// is not impersonated.
func Actor(scopes map[string]*authpb.Scope) string {
	return string(scopes[ActorScope].GetResource().GetValue())
}
