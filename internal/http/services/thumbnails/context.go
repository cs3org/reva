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

package thumbnails

import (
	"context"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

type contextKey int

const (
	// contextKeyResource is the key used to store a resource info into the context
	contextKeyResource contextKey = iota
)

// ContextSetResource adds a ResourceInfo into the context
func ContextSetResource(ctx context.Context, res *provider.ResourceInfo) context.Context {
	return context.WithValue(ctx, contextKeyResource, res)
}

// ContextMustGetResource gets a ResourceInfo from the context.
// Panics if not available.
func ContextMustGetResource(ctx context.Context) *provider.ResourceInfo {
	v, ok := ctx.Value(contextKeyResource).(*provider.ResourceInfo)
	if !ok {
		panic("resource not in context")
	}
	return v
}
