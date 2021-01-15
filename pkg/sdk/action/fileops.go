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
	p "path"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	storage "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"

	"github.com/cs3org/reva/pkg/sdk"
	"github.com/cs3org/reva/pkg/sdk/common/net"
)

// FileOperationsAction offers basic file operations.
type FileOperationsAction struct {
	action
}

// Stat queries the file information of the specified remote resource.
func (action *FileOperationsAction) Stat(path string) (*storage.ResourceInfo, error) {
	ref := &provider.Reference{
		Spec: &provider.Reference_Path{Path: path},
	}
	req := &provider.StatRequest{Ref: ref}
	res, err := action.session.Client().Stat(action.session.Context(), req)
	if err := net.CheckRPCInvocation("querying resource information", res, err); err != nil {
		return nil, err
	}
	return res.Info, nil
}

// FileExists checks whether the specified file exists.
func (action *FileOperationsAction) FileExists(path string) bool {
	// Stat the file and see if that succeeds; if so, check if the resource is indeed a file
	info, err := action.Stat(path)
	if err != nil {
		return false
	}
	return info.Type == provider.ResourceType_RESOURCE_TYPE_FILE
}

// DirExists checks whether the specified directory exists.
func (action *FileOperationsAction) DirExists(path string) bool {
	// Stat the file and see if that succeeds; if so, check if the resource is indeed a directory
	info, err := action.Stat(path)
	if err != nil {
		return false
	}
	return info.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER
}

// ResourceExists checks whether the specified resource exists (w/o checking for its actual type).
func (action *FileOperationsAction) ResourceExists(path string) bool {
	// Stat the file and see if that succeeds
	_, err := action.Stat(path)
	return err == nil
}

// MakePath creates the entire directory tree specified by the given path.
func (action *FileOperationsAction) MakePath(path string) error {
	path = strings.TrimPrefix(path, "/")

	var curPath string
	for _, token := range strings.Split(path, "/") {
		curPath = p.Join(curPath, "/"+token)

		fileInfo, err := action.Stat(curPath)
		if err != nil { // Stating failed, so the path probably doesn't exist yet
			ref := &provider.Reference{
				Spec: &provider.Reference_Path{Path: curPath},
			}
			req := &provider.CreateContainerRequest{Ref: ref}
			res, err := action.session.Client().CreateContainer(action.session.Context(), req)
			if err := net.CheckRPCInvocation("creating container", res, err); err != nil {
				return err
			}
		} else if fileInfo.Type != provider.ResourceType_RESOURCE_TYPE_CONTAINER {
			// The path exists, so make sure that is actually a directory
			return fmt.Errorf("'%v' is not a directory", curPath)
		}
	}

	return nil
}

// Move moves the specified source to a new location. The caller must ensure that the target directory exists.
func (action *FileOperationsAction) Move(source string, target string) error {
	sourceRef := &provider.Reference{
		Spec: &provider.Reference_Path{Path: source},
	}
	targetRef := &provider.Reference{
		Spec: &provider.Reference_Path{Path: target},
	}
	req := &provider.MoveRequest{Source: sourceRef, Destination: targetRef}
	res, err := action.session.Client().Move(action.session.Context(), req)
	if err := net.CheckRPCInvocation("moving resource", res, err); err != nil {
		return err
	}

	return nil
}

// MoveTo moves the specified source to the target directory, creating it if necessary.
func (action *FileOperationsAction) MoveTo(source string, path string) error {
	if err := action.MakePath(path); err != nil {
		return fmt.Errorf("unable to create the target directory '%v': %v", path, err)
	}

	path = p.Join(path, p.Base(source)) // Keep the original resource base name
	return action.Move(source, path)
}

// Remove deletes the specified resource.
func (action *FileOperationsAction) Remove(path string) error {
	ref := &provider.Reference{
		Spec: &provider.Reference_Path{Path: path},
	}
	req := &provider.DeleteRequest{Ref: ref}
	res, err := action.session.Client().Delete(action.session.Context(), req)
	if err := net.CheckRPCInvocation("deleting resource", res, err); err != nil {
		return err
	}

	return nil
}

// NewFileOperationsAction creates a new file operations action.
func NewFileOperationsAction(session *sdk.Session) (*FileOperationsAction, error) {
	action := &FileOperationsAction{}
	if err := action.initAction(session); err != nil {
		return nil, fmt.Errorf("unable to create the FileOperationsAction: %v", err)
	}
	return action, nil
}

// MustNewFileOperationsAction creates a new file operations action and panics on failure.
func MustNewFileOperationsAction(session *sdk.Session) *FileOperationsAction {
	action, err := NewFileOperationsAction(session)
	if err != nil {
		panic(err)
	}
	return action
}
