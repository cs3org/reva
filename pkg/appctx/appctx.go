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

package appctx

import (
	"context"
	"unsafe"

	"github.com/rs/zerolog"
)

// DeletingSharedResource flags to a storage a shared resource is being deleted not by the owner.
var DeletingSharedResource struct{}

type emptyCtx int

type valueCtx struct {
	context.Context
	key, val interface{}
}

type iface struct {
	itab, data uintptr
}

// GetKeyValues retrieves all the key-value pairs from the provided context.
func GetKeyValues(ctx context.Context) map[interface{}]interface{} {
	m := make(map[interface{}]interface{})
	getKeyValue(ctx, m)
	return m
}

// PutKeyValues puts
func PutKeyValues(m map[interface{}]interface{}) context.Context {
	ctx := context.Background()
	for key, value := range m {
		ctx = context.WithValue(ctx, key, value)
	}
	return ctx
}

func getKeyValue(ctx context.Context, m map[interface{}]interface{}) {
	ictx := *(*iface)(unsafe.Pointer(&ctx))
	if ictx.data == 0 || int(*(*emptyCtx)(unsafe.Pointer(ictx.data))) == 0 {
		return
	}
	valCtx := (*valueCtx)(unsafe.Pointer(ictx.data))
	if valCtx != nil && valCtx.key != nil {
		m[valCtx.key] = valCtx.val
	}
	getKeyValue(valCtx.Context, m)
}

// WithLogger returns a context with an associated logger.
func WithLogger(ctx context.Context, l *zerolog.Logger) context.Context {
	return l.WithContext(ctx)
}

// GetLogger returns the logger associated with the given context
// or a disabled logger in case no logger is stored inside the context.
func GetLogger(ctx context.Context) *zerolog.Logger {
	return zerolog.Ctx(ctx)
}
