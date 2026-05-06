// Copyright 2018-2024 CERN
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
	"github.com/cs3org/reva/v3/pkg/appctx"
)

type environment struct {
	userOne    *user.User
	userOneCtx context.Context

	userTwo    *user.User
	userTwoCtx context.Context

	userThree    *user.User
	userThreeCtx context.Context

	resourceIdOne *provider.ResourceId
	resourceIdTwo *provider.ResourceId
}

func createEnvironment() environment {
	userOne := &user.User{Id: &user.UserId{OpaqueId: "userOne"}}
	userTwo := &user.User{Id: &user.UserId{OpaqueId: "userTwo"}}
	userThree := &user.User{Id: &user.UserId{OpaqueId: "userThree"}}

	resourceIdOne := &provider.ResourceId{OpaqueId: "resourceInfoOne"}
	resourceIdTwo := &provider.ResourceId{OpaqueId: "resourceInfoTwo"}

	return environment{
		userOne:      userOne,
		userOneCtx:   appctx.ContextSetUser(context.Background(), userOne),
		userTwo:      userTwo,
		userTwoCtx:   appctx.ContextSetUser(context.Background(), userTwo),
		userThree:    userThree,
		userThreeCtx: appctx.ContextSetUser(context.Background(), userThree),

		resourceIdOne: resourceIdOne,
		resourceIdTwo: resourceIdTwo,
	}
}

func TestListResourcesForLabel(t *testing.T) {
	env := createEnvironment()
	sut, _ := New(nil)

	resources, _ := sut.ListResourcesForLabel(env.userOneCtx, "favorite")
	if len(resources) != 0 {
		t.Error("ListResourcesForLabel should not return anything when a user hasn't set a label")
	}

	_ = sut.SetLabel(env.userOneCtx, "favorite", env.resourceIdOne)
	_ = sut.SetLabel(env.userTwoCtx, "favorite", env.resourceIdOne)
	_ = sut.SetLabel(env.userTwoCtx, "favorite", env.resourceIdTwo)

	resources, _ = sut.ListResourcesForLabel(env.userOneCtx, "favorite")
	if len(resources) != 1 {
		t.Errorf("Expected %d resources got %d", 1, len(resources))
	}

	resources, _ = sut.ListResourcesForLabel(env.userTwoCtx, "favorite")
	if len(resources) != 2 {
		t.Errorf("Expected %d resources got %d", 2, len(resources))
	}

	resources, _ = sut.ListResourcesForLabel(env.userThreeCtx, "favorite")
	if len(resources) != 0 {
		t.Errorf("Expected %d resources got %d", 0, len(resources))
	}
}

func TestSetLabel(t *testing.T) {
	env := createEnvironment()

	sut, _ := New(nil)

	resources, _ := sut.ListResourcesForLabel(env.userOneCtx, "favorite")
	lenBefore := len(resources)

	_ = sut.SetLabel(env.userOneCtx, "favorite", env.resourceIdOne)

	resources, _ = sut.ListResourcesForLabel(env.userOneCtx, "favorite")
	lenAfter := len(resources)

	if lenAfter-lenBefore != 1 {
		t.Errorf("Setting a label should add 1 resource but actually added %d", lenAfter-lenBefore)
	}
}

func TestUnsetLabel(t *testing.T) {
	env := createEnvironment()

	sut, _ := New(nil)

	_ = sut.SetLabel(env.userOneCtx, "favorite", env.resourceIdOne)
	resources, _ := sut.ListResourcesForLabel(env.userOneCtx, "favorite")
	lenBefore := len(resources)

	_ = sut.UnsetLabel(env.userOneCtx, "favorite", env.resourceIdOne)

	resources, _ = sut.ListResourcesForLabel(env.userOneCtx, "favorite")
	lenAfter := len(resources)

	if lenAfter-lenBefore != -1 {
		t.Errorf("Unsetting a label should remove 1 label but actually removed %d", lenAfter-lenBefore)
	}
}
