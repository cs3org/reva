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

package grpc

import (
	"context"
	"errors"
	"path"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
)

type files struct{ b *Backend }

func (f files) Stat(ctx context.Context, ref *provider.Reference) (*provider.ResourceInfo, error) {
	res, err := call(ctx, f.b, gateway.GatewayAPIClient.Stat, &provider.StatRequest{Ref: ref})
	if err != nil {
		return nil, err
	}
	return res.Info, nil
}

func (f files) List(ctx context.Context, ref *provider.Reference) ([]*provider.ResourceInfo, error) {
	res, err := call(ctx, f.b, gateway.GatewayAPIClient.ListContainer, &provider.ListContainerRequest{Ref: ref})
	if err != nil {
		return nil, err
	}
	return res.Infos, nil
}

func (f files) Walk(ctx context.Context, ref *provider.Reference, fn func(*provider.ResourceInfo) error) error {
	info, err := f.Stat(ctx, ref)
	if err != nil {
		return err
	}
	return f.walk(ctx, info, fn)
}

func (f files) walk(ctx context.Context, info *provider.ResourceInfo, fn func(*provider.ResourceInfo) error) error {
	if err := fn(info); err != nil || info.Type != provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		return err
	}
	children, err := f.List(ctx, &provider.Reference{Path: info.Path})
	if err != nil {
		return err
	}
	for _, child := range children {
		if err := f.walk(ctx, child, fn); err != nil {
			return err
		}
	}
	return nil
}

func (f files) Mkdir(ctx context.Context, ref *provider.Reference, parents bool) error {
	c, gctx, err := f.b.gw(ctx)
	if err != nil {
		return err
	}
	_, err = check(c.CreateContainer(gctx, &provider.CreateContainerRequest{Ref: ref}))
	if err == nil || !parents {
		return err
	}
	se, ok := errors.AsType[*backend.StatusError](err)
	switch {
	case ok && se.Code == rpc.Code_CODE_ALREADY_EXISTS:
		return nil
	case ok && ref.ResourceId == nil && (se.Code == rpc.Code_CODE_NOT_FOUND || se.Code == rpc.Code_CODE_FAILED_PRECONDITION):
		// The parent is missing: the providers do not agree on the code.
		parent := path.Dir(ref.Path)
		if parent == ref.Path {
			return err
		}
		if perr := f.Mkdir(ctx, &provider.Reference{Path: parent}, true); perr != nil {
			return perr
		}
		_, err = check(c.CreateContainer(gctx, &provider.CreateContainerRequest{Ref: ref}))
	}
	return err
}

// Move follows the Overwrite header of WebDAV (ocdav): without overwrite a
// dst that exists is an error, with overwrite it is deleted first.
func (f files) Move(ctx context.Context, src, dst *provider.Reference, overwrite bool) error {
	c, ctx, err := f.b.gw(ctx)
	if err != nil {
		return err
	}
	_, err = check(c.Stat(ctx, &provider.StatRequest{Ref: dst}))
	switch {
	case err == nil && !overwrite:
		return &backend.StatusError{Code: rpc.Code_CODE_ALREADY_EXISTS, Msg: "the destination exists"}
	case err == nil:
		if _, err := check(c.Delete(ctx, &provider.DeleteRequest{Ref: dst})); err != nil {
			return err
		}
	case !backend.IsNotFound(err):
		return err
	}
	_, err = check(c.Move(ctx, &provider.MoveRequest{Source: src, Destination: dst}))
	return err
}

// Copy is not in CS3: WebDAV COPY does it with a download and an upload.
func (f files) Copy(ctx context.Context, src, dst *provider.Reference, overwrite bool) error {
	return notAvailable("copy")
}

func (f files) Remove(ctx context.Context, ref *provider.Reference) error {
	_, err := call(ctx, f.b, gateway.GatewayAPIClient.Delete, &provider.DeleteRequest{Ref: ref})
	return err
}

func (f files) Touch(ctx context.Context, ref *provider.Reference) error {
	_, err := call(ctx, f.b, gateway.GatewayAPIClient.TouchFile, &provider.TouchFileRequest{Ref: ref})
	return err
}

func (f files) ResolveSpace(ctx context.Context, alias string) (*provider.Reference, error) {
	return nil, notAvailable("space lookup")
}
