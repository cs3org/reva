// Copyright 2018-2022 CERN
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

// Code generated by mockery v2.40.2. DO NOT EDIT.

package mocks

import (
	context "context"
	fs "io/fs"

	io "io"

	mock "github.com/stretchr/testify/mock"

	node "github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/node"
)

// Tree is an autogenerated mock type for the Tree type
type Tree struct {
	mock.Mock
}

type Tree_Expecter struct {
	mock *mock.Mock
}

func (_m *Tree) EXPECT() *Tree_Expecter {
	return &Tree_Expecter{mock: &_m.Mock}
}

// CreateDir provides a mock function with given fields: ctx, _a1
func (_m *Tree) CreateDir(ctx context.Context, _a1 *node.Node) error {
	ret := _m.Called(ctx, _a1)

	if len(ret) == 0 {
		panic("no return value specified for CreateDir")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, *node.Node) error); ok {
		r0 = rf(ctx, _a1)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Tree_CreateDir_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'CreateDir'
type Tree_CreateDir_Call struct {
	*mock.Call
}

// CreateDir is a helper method to define mock.On call
//   - ctx context.Context
//   - _a1 *node.Node
func (_e *Tree_Expecter) CreateDir(ctx interface{}, _a1 interface{}) *Tree_CreateDir_Call {
	return &Tree_CreateDir_Call{Call: _e.mock.On("CreateDir", ctx, _a1)}
}

func (_c *Tree_CreateDir_Call) Run(run func(ctx context.Context, _a1 *node.Node)) *Tree_CreateDir_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(*node.Node))
	})
	return _c
}

func (_c *Tree_CreateDir_Call) Return(err error) *Tree_CreateDir_Call {
	_c.Call.Return(err)
	return _c
}

func (_c *Tree_CreateDir_Call) RunAndReturn(run func(context.Context, *node.Node) error) *Tree_CreateDir_Call {
	_c.Call.Return(run)
	return _c
}

// Delete provides a mock function with given fields: ctx, _a1
func (_m *Tree) Delete(ctx context.Context, _a1 *node.Node) error {
	ret := _m.Called(ctx, _a1)

	if len(ret) == 0 {
		panic("no return value specified for Delete")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, *node.Node) error); ok {
		r0 = rf(ctx, _a1)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Tree_Delete_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Delete'
type Tree_Delete_Call struct {
	*mock.Call
}

// Delete is a helper method to define mock.On call
//   - ctx context.Context
//   - _a1 *node.Node
func (_e *Tree_Expecter) Delete(ctx interface{}, _a1 interface{}) *Tree_Delete_Call {
	return &Tree_Delete_Call{Call: _e.mock.On("Delete", ctx, _a1)}
}

func (_c *Tree_Delete_Call) Run(run func(ctx context.Context, _a1 *node.Node)) *Tree_Delete_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(*node.Node))
	})
	return _c
}

func (_c *Tree_Delete_Call) Return(err error) *Tree_Delete_Call {
	_c.Call.Return(err)
	return _c
}

func (_c *Tree_Delete_Call) RunAndReturn(run func(context.Context, *node.Node) error) *Tree_Delete_Call {
	_c.Call.Return(run)
	return _c
}

// DeleteBlob provides a mock function with given fields: spaceID, blobId
func (_m *Tree) DeleteBlob(spaceID string, blobId string) error {
	ret := _m.Called(spaceID, blobId)

	if len(ret) == 0 {
		panic("no return value specified for DeleteBlob")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string) error); ok {
		r0 = rf(spaceID, blobId)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Tree_DeleteBlob_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'DeleteBlob'
type Tree_DeleteBlob_Call struct {
	*mock.Call
}

// DeleteBlob is a helper method to define mock.On call
//   - spaceID string
//   - blobId string
func (_e *Tree_Expecter) DeleteBlob(spaceID interface{}, blobId interface{}) *Tree_DeleteBlob_Call {
	return &Tree_DeleteBlob_Call{Call: _e.mock.On("DeleteBlob", spaceID, blobId)}
}

func (_c *Tree_DeleteBlob_Call) Run(run func(spaceID string, blobId string)) *Tree_DeleteBlob_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(string), args[1].(string))
	})
	return _c
}

func (_c *Tree_DeleteBlob_Call) Return(_a0 error) *Tree_DeleteBlob_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *Tree_DeleteBlob_Call) RunAndReturn(run func(string, string) error) *Tree_DeleteBlob_Call {
	_c.Call.Return(run)
	return _c
}

// GetMD provides a mock function with given fields: ctx, _a1
func (_m *Tree) GetMD(ctx context.Context, _a1 *node.Node) (fs.FileInfo, error) {
	ret := _m.Called(ctx, _a1)

	if len(ret) == 0 {
		panic("no return value specified for GetMD")
	}

	var r0 fs.FileInfo
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *node.Node) (fs.FileInfo, error)); ok {
		return rf(ctx, _a1)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *node.Node) fs.FileInfo); ok {
		r0 = rf(ctx, _a1)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(fs.FileInfo)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *node.Node) error); ok {
		r1 = rf(ctx, _a1)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Tree_GetMD_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetMD'
type Tree_GetMD_Call struct {
	*mock.Call
}

// GetMD is a helper method to define mock.On call
//   - ctx context.Context
//   - _a1 *node.Node
func (_e *Tree_Expecter) GetMD(ctx interface{}, _a1 interface{}) *Tree_GetMD_Call {
	return &Tree_GetMD_Call{Call: _e.mock.On("GetMD", ctx, _a1)}
}

func (_c *Tree_GetMD_Call) Run(run func(ctx context.Context, _a1 *node.Node)) *Tree_GetMD_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(*node.Node))
	})
	return _c
}

func (_c *Tree_GetMD_Call) Return(_a0 fs.FileInfo, _a1 error) *Tree_GetMD_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *Tree_GetMD_Call) RunAndReturn(run func(context.Context, *node.Node) (fs.FileInfo, error)) *Tree_GetMD_Call {
	_c.Call.Return(run)
	return _c
}

// ListFolder provides a mock function with given fields: ctx, _a1
func (_m *Tree) ListFolder(ctx context.Context, _a1 *node.Node) ([]*node.Node, error) {
	ret := _m.Called(ctx, _a1)

	if len(ret) == 0 {
		panic("no return value specified for ListFolder")
	}

	var r0 []*node.Node
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *node.Node) ([]*node.Node, error)); ok {
		return rf(ctx, _a1)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *node.Node) []*node.Node); ok {
		r0 = rf(ctx, _a1)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*node.Node)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *node.Node) error); ok {
		r1 = rf(ctx, _a1)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Tree_ListFolder_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'ListFolder'
type Tree_ListFolder_Call struct {
	*mock.Call
}

// ListFolder is a helper method to define mock.On call
//   - ctx context.Context
//   - _a1 *node.Node
func (_e *Tree_Expecter) ListFolder(ctx interface{}, _a1 interface{}) *Tree_ListFolder_Call {
	return &Tree_ListFolder_Call{Call: _e.mock.On("ListFolder", ctx, _a1)}
}

func (_c *Tree_ListFolder_Call) Run(run func(ctx context.Context, _a1 *node.Node)) *Tree_ListFolder_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(*node.Node))
	})
	return _c
}

func (_c *Tree_ListFolder_Call) Return(_a0 []*node.Node, _a1 error) *Tree_ListFolder_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *Tree_ListFolder_Call) RunAndReturn(run func(context.Context, *node.Node) ([]*node.Node, error)) *Tree_ListFolder_Call {
	_c.Call.Return(run)
	return _c
}

// Move provides a mock function with given fields: ctx, oldNode, newNode
func (_m *Tree) Move(ctx context.Context, oldNode *node.Node, newNode *node.Node) error {
	ret := _m.Called(ctx, oldNode, newNode)

	if len(ret) == 0 {
		panic("no return value specified for Move")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, *node.Node, *node.Node) error); ok {
		r0 = rf(ctx, oldNode, newNode)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Tree_Move_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Move'
type Tree_Move_Call struct {
	*mock.Call
}

// Move is a helper method to define mock.On call
//   - ctx context.Context
//   - oldNode *node.Node
//   - newNode *node.Node
func (_e *Tree_Expecter) Move(ctx interface{}, oldNode interface{}, newNode interface{}) *Tree_Move_Call {
	return &Tree_Move_Call{Call: _e.mock.On("Move", ctx, oldNode, newNode)}
}

func (_c *Tree_Move_Call) Run(run func(ctx context.Context, oldNode *node.Node, newNode *node.Node)) *Tree_Move_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(*node.Node), args[2].(*node.Node))
	})
	return _c
}

func (_c *Tree_Move_Call) Return(err error) *Tree_Move_Call {
	_c.Call.Return(err)
	return _c
}

func (_c *Tree_Move_Call) RunAndReturn(run func(context.Context, *node.Node, *node.Node) error) *Tree_Move_Call {
	_c.Call.Return(run)
	return _c
}

// Propagate provides a mock function with given fields: ctx, _a1, sizeDiff
func (_m *Tree) Propagate(ctx context.Context, _a1 *node.Node, sizeDiff int64) error {
	ret := _m.Called(ctx, _a1, sizeDiff)

	if len(ret) == 0 {
		panic("no return value specified for Propagate")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, *node.Node, int64) error); ok {
		r0 = rf(ctx, _a1, sizeDiff)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Tree_Propagate_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Propagate'
type Tree_Propagate_Call struct {
	*mock.Call
}

// Propagate is a helper method to define mock.On call
//   - ctx context.Context
//   - _a1 *node.Node
//   - sizeDiff int64
func (_e *Tree_Expecter) Propagate(ctx interface{}, _a1 interface{}, sizeDiff interface{}) *Tree_Propagate_Call {
	return &Tree_Propagate_Call{Call: _e.mock.On("Propagate", ctx, _a1, sizeDiff)}
}

func (_c *Tree_Propagate_Call) Run(run func(ctx context.Context, _a1 *node.Node, sizeDiff int64)) *Tree_Propagate_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(*node.Node), args[2].(int64))
	})
	return _c
}

func (_c *Tree_Propagate_Call) Return(err error) *Tree_Propagate_Call {
	_c.Call.Return(err)
	return _c
}

func (_c *Tree_Propagate_Call) RunAndReturn(run func(context.Context, *node.Node, int64) error) *Tree_Propagate_Call {
	_c.Call.Return(run)
	return _c
}

// PurgeRecycleItemFunc provides a mock function with given fields: ctx, spaceid, key, purgePath
func (_m *Tree) PurgeRecycleItemFunc(ctx context.Context, spaceid string, key string, purgePath string) (*node.Node, func() error, error) {
	ret := _m.Called(ctx, spaceid, key, purgePath)

	if len(ret) == 0 {
		panic("no return value specified for PurgeRecycleItemFunc")
	}

	var r0 *node.Node
	var r1 func() error
	var r2 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string, string) (*node.Node, func() error, error)); ok {
		return rf(ctx, spaceid, key, purgePath)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, string, string) *node.Node); ok {
		r0 = rf(ctx, spaceid, key, purgePath)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*node.Node)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, string, string) func() error); ok {
		r1 = rf(ctx, spaceid, key, purgePath)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(func() error)
		}
	}

	if rf, ok := ret.Get(2).(func(context.Context, string, string, string) error); ok {
		r2 = rf(ctx, spaceid, key, purgePath)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// Tree_PurgeRecycleItemFunc_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'PurgeRecycleItemFunc'
type Tree_PurgeRecycleItemFunc_Call struct {
	*mock.Call
}

// PurgeRecycleItemFunc is a helper method to define mock.On call
//   - ctx context.Context
//   - spaceid string
//   - key string
//   - purgePath string
func (_e *Tree_Expecter) PurgeRecycleItemFunc(ctx interface{}, spaceid interface{}, key interface{}, purgePath interface{}) *Tree_PurgeRecycleItemFunc_Call {
	return &Tree_PurgeRecycleItemFunc_Call{Call: _e.mock.On("PurgeRecycleItemFunc", ctx, spaceid, key, purgePath)}
}

func (_c *Tree_PurgeRecycleItemFunc_Call) Run(run func(ctx context.Context, spaceid string, key string, purgePath string)) *Tree_PurgeRecycleItemFunc_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(string), args[2].(string), args[3].(string))
	})
	return _c
}

func (_c *Tree_PurgeRecycleItemFunc_Call) Return(_a0 *node.Node, _a1 func() error, _a2 error) *Tree_PurgeRecycleItemFunc_Call {
	_c.Call.Return(_a0, _a1, _a2)
	return _c
}

func (_c *Tree_PurgeRecycleItemFunc_Call) RunAndReturn(run func(context.Context, string, string, string) (*node.Node, func() error, error)) *Tree_PurgeRecycleItemFunc_Call {
	_c.Call.Return(run)
	return _c
}

// ReadBlob provides a mock function with given fields: spaceID, blobId, blobSize
func (_m *Tree) ReadBlob(spaceID string, blobId string, blobSize int64) (io.ReadCloser, error) {
	ret := _m.Called(spaceID, blobId, blobSize)

	if len(ret) == 0 {
		panic("no return value specified for ReadBlob")
	}

	var r0 io.ReadCloser
	var r1 error
	if rf, ok := ret.Get(0).(func(string, string, int64) (io.ReadCloser, error)); ok {
		return rf(spaceID, blobId, blobSize)
	}
	if rf, ok := ret.Get(0).(func(string, string, int64) io.ReadCloser); ok {
		r0 = rf(spaceID, blobId, blobSize)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(io.ReadCloser)
		}
	}

	if rf, ok := ret.Get(1).(func(string, string, int64) error); ok {
		r1 = rf(spaceID, blobId, blobSize)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Tree_ReadBlob_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'ReadBlob'
type Tree_ReadBlob_Call struct {
	*mock.Call
}

// ReadBlob is a helper method to define mock.On call
//   - spaceID string
//   - blobId string
//   - blobSize int64
func (_e *Tree_Expecter) ReadBlob(spaceID interface{}, blobId interface{}, blobSize interface{}) *Tree_ReadBlob_Call {
	return &Tree_ReadBlob_Call{Call: _e.mock.On("ReadBlob", spaceID, blobId, blobSize)}
}

func (_c *Tree_ReadBlob_Call) Run(run func(spaceID string, blobId string, blobSize int64)) *Tree_ReadBlob_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(string), args[1].(string), args[2].(int64))
	})
	return _c
}

func (_c *Tree_ReadBlob_Call) Return(_a0 io.ReadCloser, _a1 error) *Tree_ReadBlob_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *Tree_ReadBlob_Call) RunAndReturn(run func(string, string, int64) (io.ReadCloser, error)) *Tree_ReadBlob_Call {
	_c.Call.Return(run)
	return _c
}

// RestoreRecycleItemFunc provides a mock function with given fields: ctx, spaceid, key, trashPath, target
func (_m *Tree) RestoreRecycleItemFunc(ctx context.Context, spaceid string, key string, trashPath string, target *node.Node) (*node.Node, *node.Node, func() error, error) {
	ret := _m.Called(ctx, spaceid, key, trashPath, target)

	if len(ret) == 0 {
		panic("no return value specified for RestoreRecycleItemFunc")
	}

	var r0 *node.Node
	var r1 *node.Node
	var r2 func() error
	var r3 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string, string, *node.Node) (*node.Node, *node.Node, func() error, error)); ok {
		return rf(ctx, spaceid, key, trashPath, target)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, string, string, *node.Node) *node.Node); ok {
		r0 = rf(ctx, spaceid, key, trashPath, target)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*node.Node)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, string, string, *node.Node) *node.Node); ok {
		r1 = rf(ctx, spaceid, key, trashPath, target)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*node.Node)
		}
	}

	if rf, ok := ret.Get(2).(func(context.Context, string, string, string, *node.Node) func() error); ok {
		r2 = rf(ctx, spaceid, key, trashPath, target)
	} else {
		if ret.Get(2) != nil {
			r2 = ret.Get(2).(func() error)
		}
	}

	if rf, ok := ret.Get(3).(func(context.Context, string, string, string, *node.Node) error); ok {
		r3 = rf(ctx, spaceid, key, trashPath, target)
	} else {
		r3 = ret.Error(3)
	}

	return r0, r1, r2, r3
}

// Tree_RestoreRecycleItemFunc_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'RestoreRecycleItemFunc'
type Tree_RestoreRecycleItemFunc_Call struct {
	*mock.Call
}

// RestoreRecycleItemFunc is a helper method to define mock.On call
//   - ctx context.Context
//   - spaceid string
//   - key string
//   - trashPath string
//   - target *node.Node
func (_e *Tree_Expecter) RestoreRecycleItemFunc(ctx interface{}, spaceid interface{}, key interface{}, trashPath interface{}, target interface{}) *Tree_RestoreRecycleItemFunc_Call {
	return &Tree_RestoreRecycleItemFunc_Call{Call: _e.mock.On("RestoreRecycleItemFunc", ctx, spaceid, key, trashPath, target)}
}

func (_c *Tree_RestoreRecycleItemFunc_Call) Run(run func(ctx context.Context, spaceid string, key string, trashPath string, target *node.Node)) *Tree_RestoreRecycleItemFunc_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(string), args[2].(string), args[3].(string), args[4].(*node.Node))
	})
	return _c
}

func (_c *Tree_RestoreRecycleItemFunc_Call) Return(_a0 *node.Node, _a1 *node.Node, _a2 func() error, _a3 error) *Tree_RestoreRecycleItemFunc_Call {
	_c.Call.Return(_a0, _a1, _a2, _a3)
	return _c
}

func (_c *Tree_RestoreRecycleItemFunc_Call) RunAndReturn(run func(context.Context, string, string, string, *node.Node) (*node.Node, *node.Node, func() error, error)) *Tree_RestoreRecycleItemFunc_Call {
	_c.Call.Return(run)
	return _c
}

// Setup provides a mock function with given fields:
func (_m *Tree) Setup() error {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for Setup")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Tree_Setup_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Setup'
type Tree_Setup_Call struct {
	*mock.Call
}

// Setup is a helper method to define mock.On call
func (_e *Tree_Expecter) Setup() *Tree_Setup_Call {
	return &Tree_Setup_Call{Call: _e.mock.On("Setup")}
}

func (_c *Tree_Setup_Call) Run(run func()) *Tree_Setup_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *Tree_Setup_Call) Return(_a0 error) *Tree_Setup_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *Tree_Setup_Call) RunAndReturn(run func() error) *Tree_Setup_Call {
	_c.Call.Return(run)
	return _c
}

// TouchFile provides a mock function with given fields: ctx, _a1, markprocessing, mtime
func (_m *Tree) TouchFile(ctx context.Context, _a1 *node.Node, markprocessing bool, mtime string) error {
	ret := _m.Called(ctx, _a1, markprocessing, mtime)

	if len(ret) == 0 {
		panic("no return value specified for TouchFile")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, *node.Node, bool, string) error); ok {
		r0 = rf(ctx, _a1, markprocessing, mtime)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Tree_TouchFile_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'TouchFile'
type Tree_TouchFile_Call struct {
	*mock.Call
}

// TouchFile is a helper method to define mock.On call
//   - ctx context.Context
//   - _a1 *node.Node
//   - markprocessing bool
//   - mtime string
func (_e *Tree_Expecter) TouchFile(ctx interface{}, _a1 interface{}, markprocessing interface{}, mtime interface{}) *Tree_TouchFile_Call {
	return &Tree_TouchFile_Call{Call: _e.mock.On("TouchFile", ctx, _a1, markprocessing, mtime)}
}

func (_c *Tree_TouchFile_Call) Run(run func(ctx context.Context, _a1 *node.Node, markprocessing bool, mtime string)) *Tree_TouchFile_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(*node.Node), args[2].(bool), args[3].(string))
	})
	return _c
}

func (_c *Tree_TouchFile_Call) Return(_a0 error) *Tree_TouchFile_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *Tree_TouchFile_Call) RunAndReturn(run func(context.Context, *node.Node, bool, string) error) *Tree_TouchFile_Call {
	_c.Call.Return(run)
	return _c
}

// WriteBlob provides a mock function with given fields: spaceID, blobId, blobSize, source
func (_m *Tree) WriteBlob(spaceID string, blobId string, blobSize int64, source string) error {
	ret := _m.Called(spaceID, blobId, blobSize, source)

	if len(ret) == 0 {
		panic("no return value specified for WriteBlob")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string, int64, string) error); ok {
		r0 = rf(spaceID, blobId, blobSize, source)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Tree_WriteBlob_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'WriteBlob'
type Tree_WriteBlob_Call struct {
	*mock.Call
}

// WriteBlob is a helper method to define mock.On call
//   - spaceID string
//   - blobId string
//   - blobSize int64
//   - source string
func (_e *Tree_Expecter) WriteBlob(spaceID interface{}, blobId interface{}, blobSize interface{}, source interface{}) *Tree_WriteBlob_Call {
	return &Tree_WriteBlob_Call{Call: _e.mock.On("WriteBlob", spaceID, blobId, blobSize, source)}
}

func (_c *Tree_WriteBlob_Call) Run(run func(spaceID string, blobId string, blobSize int64, source string)) *Tree_WriteBlob_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(string), args[1].(string), args[2].(int64), args[3].(string))
	})
	return _c
}

func (_c *Tree_WriteBlob_Call) Return(_a0 error) *Tree_WriteBlob_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *Tree_WriteBlob_Call) RunAndReturn(run func(string, string, int64, string) error) *Tree_WriteBlob_Call {
	_c.Call.Return(run)
	return _c
}

// NewTree creates a new instance of Tree. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewTree(t interface {
	mock.TestingT
	Cleanup(func())
}) *Tree {
	mock := &Tree{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
