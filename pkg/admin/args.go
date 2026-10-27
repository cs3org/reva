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

package admin

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// UserID is an invocation argument that names a user by its CS3 id, because a username
// can be reused. It takes the protojson of a UserId, or a plain opaque id so that
// "reva invoke" can be typed by hand. An empty value names no user.
type UserID struct {
	id *userpb.UserId
}

// ID returns the CS3 id, or nil for no user.
func (u UserID) ID() *userpb.UserId {
	if u.id.GetOpaqueId() == "" {
		return nil
	}
	return u.id
}

// IsZero reports whether u names no user.
func (u UserID) IsZero() bool { return u.ID() == nil }

func (u UserID) String() string { return u.id.GetOpaqueId() }

func (u *UserID) UnmarshalText(b []byte) error {
	s := strings.TrimSpace(string(b))
	if !strings.HasPrefix(s, "{") {
		*u = UserID{}
		if s != "" {
			u.id = &userpb.UserId{OpaqueId: s}
		}
		return nil
	}
	id := &userpb.UserId{}
	if err := protojson.Unmarshal([]byte(s), id); err != nil {
		return fmt.Errorf("not the protojson of a UserId: %w", err)
	}
	if id.GetOpaqueId() == "" {
		return errors.New("the UserId has no opaque_id")
	}
	*u = UserID{id: id}
	return nil
}

func (u UserID) MarshalText() ([]byte, error) {
	if u.IsZero() {
		return nil, nil
	}
	return protojson.Marshal(u.id)
}

// UserArg returns the invocation argument that names id, or "" for no user.
func UserArg(id *userpb.UserId) string {
	b, _ := UserID{id: id}.MarshalText()
	return string(b)
}

// Grantee is an invocation argument that names the grantee of a share. It takes the
// protojson of a CS3 Grantee, or a plain opaque id of a user. An empty value names no grantee.
type Grantee struct {
	g *provider.Grantee
}

// Get returns the CS3 grantee, or nil for none.
func (g Grantee) Get() *provider.Grantee {
	if granteeOpaqueID(g.g) == "" {
		return nil
	}
	return g.g
}

// IsZero reports whether g names no grantee.
func (g Grantee) IsZero() bool { return g.Get() == nil }

func (g Grantee) String() string { return granteeOpaqueID(g.g) }

func (g *Grantee) UnmarshalText(b []byte) error {
	s := strings.TrimSpace(string(b))
	if !strings.HasPrefix(s, "{") {
		*g = Grantee{}
		if s != "" {
			g.g = &provider.Grantee{Type: provider.GranteeType_GRANTEE_TYPE_USER, Id: &provider.Grantee_UserId{UserId: &userpb.UserId{OpaqueId: s}}}
		}
		return nil
	}
	pg := &provider.Grantee{}
	if err := protojson.Unmarshal([]byte(s), pg); err != nil {
		return fmt.Errorf("not the protojson of a Grantee: %w", err)
	}
	if granteeOpaqueID(pg) == "" {
		return errors.New("the Grantee has no user or group with an opaque_id")
	}
	*g = Grantee{g: pg}
	return nil
}

func (g Grantee) MarshalText() ([]byte, error) {
	if g.IsZero() {
		return nil, nil
	}
	return protojson.Marshal(g.g)
}

// GranteeArg returns the invocation argument that names g, or "" for none.
func GranteeArg(g *provider.Grantee) string {
	b, _ := Grantee{g: g}.MarshalText()
	return string(b)
}

func granteeOpaqueID(g *provider.Grantee) string {
	switch g.GetType() {
	case provider.GranteeType_GRANTEE_TYPE_USER:
		return g.GetUserId().GetOpaqueId()
	case provider.GranteeType_GRANTEE_TYPE_GROUP:
		return g.GetGroupId().GetOpaqueId()
	}
	return ""
}

// Candidates returns the users to act as, in order, without the empty and the repeated
// ones: the owner and the initiator of a share are often the same.
func Candidates(ids ...*userpb.UserId) []*userpb.UserId {
	out := make([]*userpb.UserId, 0, len(ids))
	for _, id := range ids {
		if id.GetOpaqueId() != "" && !slices.ContainsFunc(out, func(o *userpb.UserId) bool { return proto.Equal(o, id) }) {
			out = append(out, id)
		}
	}
	return out
}

// ParseResourceID reads "storage_id!opaque_id", or "storage_id$space_id!opaque_id" for the
// spaces. The share tables match the storage id and the opaque id only.
func ParseResourceID(s string) (*provider.ResourceId, error) {
	storage, opaque, ok := strings.Cut(s, "!")
	if !ok || storage == "" || opaque == "" {
		return nil, errtypes.BadRequest(fmt.Sprintf("invalid resource id %q: use storage_id!opaque_id", s))
	}
	storage, space, _ := strings.Cut(storage, "$")
	return &provider.ResourceId{StorageId: storage, SpaceId: space, OpaqueId: opaque}, nil
}
