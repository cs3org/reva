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

package usermapper

import (
	"context"
	"fmt"
	"os/user"
	"runtime"
	"strconv"

	"golang.org/x/sys/unix"

	revactx "github.com/cs3org/reva/v2/pkg/ctx"
)

type Mapper interface {
	RunInBaseScope(f func() error) error
	MapUser(username string) (int, int, error)
	ScopeBase() (func() error, error)
	ScopeUser(ctx context.Context) (func() error, error)
	ScopeUserByIds(uid, gid int) (func() error, error)
}

type UnixMapper struct {
	baseUid int
}

type UnscopeFunc func() error

// New returns a new user mapper
func NewUnixMapper() *UnixMapper {
	baseUid, _ := unix.SetfsuidRetUid(-1)

	return &UnixMapper{
		baseUid: baseUid,
	}
}

// RunInUserScope runs the given function in the scope of the base user
func (um *UnixMapper) RunInBaseScope(f func() error) error {
	if um == nil {
		return f()
	}

	unscope, err := um.ScopeBase()
	if err != nil {
		return err
	}
	defer unscope()

	return f()
}

// ScopeBase returns to the base uid and gid returning a function that can be used to restore the previous scope
func (um *UnixMapper) ScopeBase() (func() error, error) {
	return um.ScopeUserByIds(um.baseUid)
}

// MapUser returns the user and group ids for the given username
func (u *UnixMapper) MapUser(username string) (int, error) {
	userDetails, err := user.Lookup(username)
	if err != nil {
		return 0, err
	}

	uid, err := strconv.Atoi(userDetails.Uid)
	if err != nil {
		return 0, err
	}

	return uid, nil
}

func (um *UnixMapper) ScopeUser(ctx context.Context) (func() error, error) {
	u := revactx.ContextMustGetUser(ctx)

	uid, err := um.MapUser(u.Username)
	if err != nil {
		return nil, err
	}
	return um.ScopeUserByIds(uid)
}
func (um *UnixMapper) ScopeUserByIds(uid int) (func() error, error) {
	runtime.LockOSThread() // Lock this Goroutine to the current OS thread

	prevUid, err := unix.SetfsuidRetUid(uid)
	if err != nil {
		return nil, err
	}
	if testUid, _ := unix.SetfsuidRetUid(-1); testUid != uid {
		return nil, fmt.Errorf("failed to setfsuid to %d", uid)
	}

	return func() error {
		_ = unix.Setfsuid(prevUid)
		runtime.UnlockOSThread()
		return nil
	}, nil
}
