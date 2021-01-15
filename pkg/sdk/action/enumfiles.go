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

package action

import (
	"fmt"

	storage "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"

	"github.com/cs3org/reva/pkg/sdk"
	"github.com/cs3org/reva/pkg/sdk/common/net"
)

// EnumFilesAction offers functions to enumerate files and directories.
type EnumFilesAction struct {
	action
}

// ListAll retrieves all files and directories contained in the provided path.
func (action *EnumFilesAction) ListAll(path string, includeSubdirectories bool) ([]*storage.ResourceInfo, error) {
	ref := &storage.Reference{
		Spec: &storage.Reference_Path{Path: path},
	}
	req := &storage.ListContainerRequest{Ref: ref}
	res, err := action.session.Client().ListContainer(action.session.Context(), req)
	if err := net.CheckRPCInvocation("listing container", res, err); err != nil {
		return nil, err
	}

	fileList := make([]*storage.ResourceInfo, 0, len(res.Infos)*64)
	for _, fi := range res.Infos {
		// Ignore resources that are neither files nor directories
		if fi.Type <= storage.ResourceType_RESOURCE_TYPE_INVALID || fi.Type >= storage.ResourceType_RESOURCE_TYPE_INTERNAL {
			continue
		}

		fileList = append(fileList, fi)

		if fi.Type == storage.ResourceType_RESOURCE_TYPE_CONTAINER && includeSubdirectories {
			subFileList, err := action.ListAll(fi.Path, includeSubdirectories)
			if err != nil {
				return nil, err
			}

			fileList = append(fileList, subFileList...)
		}
	}

	return fileList, nil
}

// ListAllWithFilter retrieves all files and directories that fulfill the provided predicate.
func (action *EnumFilesAction) ListAllWithFilter(path string, includeSubdirectories bool, filter func(*storage.ResourceInfo) bool) ([]*storage.ResourceInfo, error) {
	all, err := action.ListAll(path, includeSubdirectories)
	if err != nil {
		return nil, err
	}

	fileList := make([]*storage.ResourceInfo, 0, len(all))

	for _, fi := range all {
		// Add only those entries that fulfill the predicate
		if filter(fi) {
			fileList = append(fileList, fi)
		}
	}

	return fileList, nil
}

// ListFiles retrieves all files contained in the provided path.
func (action *EnumFilesAction) ListFiles(path string, includeSubdirectories bool) ([]*storage.ResourceInfo, error) {
	return action.ListAllWithFilter(path, includeSubdirectories, func(fi *storage.ResourceInfo) bool {
		return fi.Type == storage.ResourceType_RESOURCE_TYPE_FILE || fi.Type == storage.ResourceType_RESOURCE_TYPE_SYMLINK
	})
}

// ListDirs retrieves all directories contained in the provided path.
func (action *EnumFilesAction) ListDirs(path string, includeSubdirectories bool) ([]*storage.ResourceInfo, error) {
	return action.ListAllWithFilter(path, includeSubdirectories, func(fi *storage.ResourceInfo) bool {
		return fi.Type == storage.ResourceType_RESOURCE_TYPE_CONTAINER
	})
}

// NewEnumFilesAction creates a new enum files action.
func NewEnumFilesAction(session *sdk.Session) (*EnumFilesAction, error) {
	action := &EnumFilesAction{}
	if err := action.initAction(session); err != nil {
		return nil, fmt.Errorf("unable to create the EnumFilesAction: %v", err)
	}
	return action, nil
}

// MustNewEnumFilesAction creates a new enum files action and panics on failure.
func MustNewEnumFilesAction(session *sdk.Session) *EnumFilesAction {
	action, err := NewEnumFilesAction(session)
	if err != nil {
		panic(err)
	}
	return action
}
