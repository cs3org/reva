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
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// FallbackHelp is the paragraph of the long help of the commands with the
// owner fallback.
const FallbackHelp = "If the resource is not yours and you can be an admin, the command finds it\n" +
	"with the Admin API, tells you, and does the operation with admin rights.\n" +
	"A change asks for a confirmation first (--yes skips it). With --as, the\n" +
	"command does not do this."

// ErrNotAvailable is the answer of an admin RPC that this version of the CLI
// does not have yet.
var ErrNotAvailable = errors.New("not available yet in this version of the CLI")

// Gate is what the owner fallback needs from the runtime. *Runtime
// implements it.
type Gate interface {
	CanAdmin(ctx context.Context) bool
	AdminContext(ctx context.Context) (context.Context, error)
	Confirm(prompt string) error
	Warnf(format string, args ...any)
}

// Fallback is the context of one owner fallback.
type Fallback struct {
	Gate Gate
	// What names the resource in the messages, for example "share 4711".
	What string
}

type fallbackKey struct{}

// InFallback reports whether ctx is the context of the admin call of an
// owner fallback. The typed admin clients set fallback=true in the request
// from it, for the audit.
func InFallback(ctx context.Context) bool {
	v, _ := ctx.Value(fallbackKey{}).(bool)
	return v
}

// OnNotFound runs userCall on the user plane. Servers answer not found for a
// resource that the caller may not see. So on not found, and only for a caller
// who can be an admin, it looks the resource up with adminLookup. If it exists,
// adminCall does the operation after a confirmation. A nil adminCall means a
// read: the lookup result is the answer. In all other cases the original answer
// is returned unchanged, so that a non-admin learns nothing.
func OnNotFound[T any](ctx context.Context, f Fallback, userCall func(context.Context) (T, error),
	adminLookup func(context.Context) (T, error), adminCall func(context.Context, T) (T, error)) (T, error) {
	res, err := userCall(ctx)
	if err == nil || !IsNotFound(err) || !f.Gate.CanAdmin(ctx) {
		return res, err
	}
	actx, aerr := f.Gate.AdminContext(ctx)
	if aerr != nil {
		return res, fmt.Errorf("%w (admin lookup of %s: %v)", err, f.What, aerr)
	}
	actx = context.WithValue(actx, fallbackKey{}, true)
	found, lerr := adminLookup(actx)
	if lerr != nil {
		if IsNotFound(lerr) || status.Code(lerr) == codes.NotFound {
			return res, err
		}
		return res, fmt.Errorf("%w (admin lookup of %s: %v)", err, f.What, lerr)
	}
	if adminCall == nil {
		f.Gate.Warnf("%s is not yours: showing it with admin rights", f.What)
		return found, nil
	}
	f.Gate.Warnf("%s is not yours: changing it with admin rights", f.What)
	if err := f.Gate.Confirm("continue"); err != nil {
		var zero T
		return zero, err
	}
	return adminCall(actx, found)
}
