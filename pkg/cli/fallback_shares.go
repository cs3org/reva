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

package cli

import (
	"context"
	"errors"

	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
)

// SharesAdmin is the part of the SharesAdmin service that the owner fallback
// of `share` and `link` uses. It answers CS3 messages, like the user plane,
// so that a command prints the same table on both planes. The errors are
// gRPC errors, with NotFound for a resource that does not exist.
type SharesAdmin interface {
	GetShare(ctx context.Context, id string) (*collaboration.Share, error)
	UpdateShare(ctx context.Context, id string, u ShareUpdate) (*collaboration.Share, error)
	RemoveShare(ctx context.Context, id string, dbOnly bool) error
	GetPublicLink(ctx context.Context, ref *link.PublicShareReference) (*link.PublicShare, error)
	UpdatePublicLink(ctx context.Context, ref *link.PublicShareReference, u ShareUpdate) (*link.PublicShare, error)
	RemovePublicLink(ctx context.Context, ref *link.PublicShareReference, dbOnly bool) error
}

// ShareUpdate is a change of a share or of a link on the admin plane. A zero
// field keeps the value.
type ShareUpdate struct {
	Role string
	// Expiration is the new expiration. A zero timestamp removes it.
	Expiration *typespb.Timestamp
	// Password is for a link only. An empty password removes it.
	Password *string
}

// SharesAdmin returns the client of the SharesAdmin service. ctx is a
// context of AdminContext. The update and remove calls tell the server that
// they run in an owner fallback when InFallback(ctx) is true, for the audit.
func (r *Runtime) SharesAdmin(ctx context.Context) (SharesAdmin, error) {
	c, err := r.adminClient(ctx)
	if err != nil {
		return nil, err
	}
	return sharesAdmin{adminpb.NewSharesAdminClient(c.Conn)}, nil
}

type sharesAdmin struct{ c adminpb.SharesAdminClient }

func (a sharesAdmin) GetShare(ctx context.Context, id string) (*collaboration.Share, error) {
	s, err := a.c.GetShare(ctx, &adminpb.GetShareRequest{Id: id})
	return s.GetShare(), err
}

func (a sharesAdmin) UpdateShare(ctx context.Context, id string, u ShareUpdate) (*collaboration.Share, error) {
	exp, remove := expirationOf(u.Expiration)
	s, err := a.c.UpdateShare(ctx, &adminpb.UpdateShareRequest{
		Id: id, Role: u.Role, Expiration: exp, RemoveExpiration: remove, Fallback: InFallback(ctx),
	})
	return s.GetShare(), err
}

func (a sharesAdmin) RemoveShare(ctx context.Context, id string, dbOnly bool) error {
	_, err := a.c.RemoveShare(ctx, &adminpb.RemoveShareRequest{Id: id, DbOnly: dbOnly, Fallback: InFallback(ctx)})
	return err
}

func (a sharesAdmin) GetPublicLink(ctx context.Context, ref *link.PublicShareReference) (*link.PublicShare, error) {
	r, err := PublicLinkRef(ref)
	if err != nil {
		return nil, err
	}
	l, err := a.c.GetPublicLink(ctx, &adminpb.GetPublicLinkRequest{Ref: r})
	return l.GetPublicShare(), err
}

func (a sharesAdmin) UpdatePublicLink(ctx context.Context, ref *link.PublicShareReference, u ShareUpdate) (*link.PublicShare, error) {
	r, err := PublicLinkRef(ref)
	if err != nil {
		return nil, err
	}
	exp, remove := expirationOf(u.Expiration)
	l, err := a.c.UpdatePublicLink(ctx, &adminpb.UpdatePublicLinkRequest{
		Ref: r, Role: u.Role, Expiration: exp, RemoveExpiration: remove, Password: u.Password, Fallback: InFallback(ctx),
	})
	return l.GetPublicShare(), err
}

func (a sharesAdmin) RemovePublicLink(ctx context.Context, ref *link.PublicShareReference, dbOnly bool) error {
	r, err := PublicLinkRef(ref)
	if err != nil {
		return err
	}
	_, err = a.c.RemovePublicLink(ctx, &adminpb.RemovePublicLinkRequest{Ref: r, DbOnly: dbOnly, Fallback: InFallback(ctx)})
	return err
}

// expirationOf returns the expiration fields of an admin request: a zero
// timestamp removes the expiration, as on the user plane.
func expirationOf(t *typespb.Timestamp) (*typespb.Timestamp, bool) {
	if t != nil && t.GetSeconds() == 0 {
		return nil, true
	}
	return t, false
}

// PublicLinkRef returns the admin reference of a CS3 link reference.
func PublicLinkRef(ref *link.PublicShareReference) (*adminpb.PublicLinkRef, error) {
	switch {
	case ref.GetId().GetOpaqueId() != "":
		return &adminpb.PublicLinkRef{Ref: &adminpb.PublicLinkRef_Id{Id: ref.GetId().GetOpaqueId()}}, nil
	case ref.GetToken() != "":
		return &adminpb.PublicLinkRef{Ref: &adminpb.PublicLinkRef_Token{Token: ref.GetToken()}}, nil
	}
	return nil, errors.New("a link reference needs an id or a token")
}
