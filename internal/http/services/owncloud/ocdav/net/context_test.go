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

package net_test

import (
	"context"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocdav/net"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Net", func() {
	var (
		alice = &userpb.User{
			Id: &userpb.UserId{
				OpaqueId: "alice",
			},
			Username: "alice",
		}
		bob = &userpb.User{
			Id: &userpb.UserId{
				OpaqueId: "bob",
			},
			Username: "bob",
		}
		aliceCtx = ctxpkg.ContextSetUser(context.Background(), alice)
		bobCtx   = ctxpkg.ContextSetUser(context.Background(), bob)
	)

	Describe("IsCurrentUserOwner", func() {
		It("returns true", func() {
			Expect(net.IsCurrentUserOwner(aliceCtx, alice.Id)).To(BeTrue())
		})

		It("returns false", func() {
			Expect(net.IsCurrentUserOwner(bobCtx, alice.Id)).To(BeFalse())
		})
	})
})
