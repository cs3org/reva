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

package plugin

import (
	"context"
	"fmt"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
)

// Ctx represents context to be passed to the plugins
type Ctx struct {
	User  *userpb.User
	Token string
}

// GetContextStruct retrieves context KV pairs and stores it into Ctx
func GetContextStruct(ctx context.Context) (*Ctx, error) {
	var ok bool
	ctxVal := &Ctx{}
	ctxVal.User, ok = ctxpkg.ContextGetUser(ctx)
	if !ok {
		return nil, fmt.Errorf("cannot get user context")
	}
	ctxVal.Token, ok = ctxpkg.ContextGetToken(ctx)
	if !ok {
		return nil, fmt.Errorf("cannot get token context")
	}
	return ctxVal, nil
}

// SetContext sets the context
func SetContext(ctxStruct *Ctx) context.Context {
	ctx := context.Background()
	ctx = ctxpkg.ContextSetUser(ctx, ctxStruct.User)
	ctx = ctxpkg.ContextSetToken(ctx, ctxStruct.Token)
	return ctx
}
