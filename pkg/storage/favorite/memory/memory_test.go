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

package memory

import (
	"context"
	"testing"

	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"

	ctxpkg "github.com/cs3org/reva/pkg/ctx"
)

type environment struct {
	userOne    *user.User
	userOneCtx context.Context

	userTwo    *user.User
	userTwoCtx context.Context

	userThree    *user.User
	userThreeCtx context.Context

	resourceInfoOne *provider.ResourceInfo
	resourceInfoTwo *provider.ResourceInfo
}

func createEnvironment() environment {
	userOne := &user.User{Id: &user.UserId{OpaqueId: "userOne"}}
	userTwo := &user.User{Id: &user.UserId{OpaqueId: "userTwo"}}
	userThree := &user.User{Id: &user.UserId{OpaqueId: "userThree"}}

	resourceInfoOne := &provider.ResourceInfo{Id: &provider.ResourceId{OpaqueId: "resourceInfoOne"}}
	resourceInfoTwo := &provider.ResourceInfo{Id: &provider.ResourceId{OpaqueId: "resourceInfoTwo"}}

	return environment{
		userOne:      userOne,
		userOneCtx:   ctxpkg.ContextSetUser(context.Background(), userOne),
		userTwo:      userTwo,
		userTwoCtx:   ctxpkg.ContextSetUser(context.Background(), userTwo),
		userThree:    userThree,
		userThreeCtx: ctxpkg.ContextSetUser(context.Background(), userThree),

		resourceInfoOne: resourceInfoOne,
		resourceInfoTwo: resourceInfoTwo,
	}
}

func TestListFavorite(t *testing.T) {
	env := createEnvironment()
	sut, _ := New(nil)

	favorites, _ := sut.ListFavorites(env.userOneCtx, env.userOne.Id)
	if len(favorites) != 0 {
		t.Error("ListFavorites should not return anything when a user hasn't set a favorite")
	}

	_ = sut.SetFavorite(env.userOneCtx, env.userOne.Id, env.resourceInfoOne)
	_ = sut.SetFavorite(env.userTwoCtx, env.userTwo.Id, env.resourceInfoOne)
	_ = sut.SetFavorite(env.userTwoCtx, env.userTwo.Id, env.resourceInfoTwo)

	favorites, _ = sut.ListFavorites(env.userOneCtx, env.userOne.Id)
	if len(favorites) != 1 {
		t.Errorf("Expected %d favorites got %d", 1, len(favorites))
	}

	favorites, _ = sut.ListFavorites(env.userTwoCtx, env.userTwo.Id)
	if len(favorites) != 2 {
		t.Errorf("Expected %d favorites got %d", 2, len(favorites))
	}

	favorites, _ = sut.ListFavorites(env.userThreeCtx, env.userThree.Id)
	if len(favorites) != 0 {
		t.Errorf("Expected %d favorites got %d", 0, len(favorites))
	}
}

func TestSetFavorite(t *testing.T) {
	env := createEnvironment()

	sut, _ := New(nil)

	favorites, _ := sut.ListFavorites(env.userOneCtx, env.userOne.Id)
	lenBefore := len(favorites)

	_ = sut.SetFavorite(env.userOneCtx, env.userOne.Id, env.resourceInfoOne)

	favorites, _ = sut.ListFavorites(env.userOneCtx, env.userOne.Id)
	lenAfter := len(favorites)

	if lenAfter-lenBefore != 1 {
		t.Errorf("Setting a favorite should add 1 favorite but actually added %d", lenAfter-lenBefore)
	}
}

func TestUnsetFavorite(t *testing.T) {
	env := createEnvironment()

	sut, _ := New(nil)

	_ = sut.SetFavorite(env.userOneCtx, env.userOne.Id, env.resourceInfoOne)
	favorites, _ := sut.ListFavorites(env.userOneCtx, env.userOne.Id)
	lenBefore := len(favorites)

	_ = sut.UnsetFavorite(env.userOneCtx, env.userOne.Id, env.resourceInfoOne)

	favorites, _ = sut.ListFavorites(env.userOneCtx, env.userOne.Id)
	lenAfter := len(favorites)

	if lenAfter-lenBefore != -1 {
		t.Errorf("Setting a favorite should remove 1 favorite but actually removed %d", lenAfter-lenBefore)
	}
}
