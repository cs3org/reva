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

package walker

import (
	"context"
	"fmt"
	"path/filepath"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"

	"github.com/cs3org/reva/pkg/errtypes"
)

// WalkFunc is the type of function called by Walk to visit each file or directory
//
// Each time the Walk function meet a file/folder path is set to the full path of this.
// The err argument reports an error related to the path, and the function can decide the action to
// do with this.
//
// The error result returned by the function controls how Walk continues. If the function returns the special value SkipDir, Walk skips the current directory.
// Otherwise, if the function returns a non-nil error, Walk stops entirely and returns that error.
type WalkFunc func(path string, info *provider.ResourceInfo, err error) error

// Walk walks the file tree rooted at root, calling fn for each file or folder in the tree, including the root.
func Walk(ctx context.Context, root string, gtw gateway.GatewayAPIClient, fn WalkFunc) error {
	info, err := stat(ctx, root, gtw)

	if err != nil {
		return fn(root, nil, err)
	}

	err = walkRecursively(ctx, root, info, gtw, fn)

	if err == filepath.SkipDir {
		return nil
	}

	return err
}

func walkRecursively(ctx context.Context, path string, info *provider.ResourceInfo, gtw gateway.GatewayAPIClient, fn WalkFunc) error {

	if info.Type != provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		return fn(path, info, nil)
	}

	list, err := readDir(ctx, path, gtw)
	errFn := fn(path, info, err)

	if err != nil || errFn != nil {
		return errFn
	}

	for _, file := range list {
		err = walkRecursively(ctx, file.Path, file, gtw, fn)
		if err != nil && (file.Type != provider.ResourceType_RESOURCE_TYPE_CONTAINER || err != filepath.SkipDir) {
			return err
		}
	}

	return nil
}

func readDir(ctx context.Context, path string, gtw gateway.GatewayAPIClient) ([]*provider.ResourceInfo, error) {
	resp, err := gtw.ListContainer(ctx, &provider.ListContainerRequest{
		Ref: &provider.Reference{
			Path: path,
		},
	})

	switch {
	case err != nil:
		return nil, err
	case resp.Status.Code == rpc.Code_CODE_NOT_FOUND:
		return nil, errtypes.NotFound(path)
	case resp.Status.Code != rpc.Code_CODE_OK:
		return nil, errtypes.InternalError(fmt.Sprintf("error reading dir %s", path))
	}

	return resp.Infos, nil
}

func stat(ctx context.Context, path string, gtw gateway.GatewayAPIClient) (*provider.ResourceInfo, error) {
	resp, err := gtw.Stat(ctx, &provider.StatRequest{
		Ref: &provider.Reference{
			Path: path,
		},
	})

	switch {
	case err != nil:
		return nil, err
	case resp.Status.Code == rpc.Code_CODE_NOT_FOUND:
		return nil, errtypes.NotFound(path)
	case resp.Status.Code != rpc.Code_CODE_OK:
		return nil, errtypes.InternalError(fmt.Sprintf("error getting stats from %s", path))
	}

	return resp.Info, nil
}
