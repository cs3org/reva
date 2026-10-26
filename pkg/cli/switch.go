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
	"fmt"
	"time"

	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	authapp "github.com/cs3org/go-cs3apis/cs3/auth/applications/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// switcher sends a call to the HTTPS backend when the primary backend
// cannot do it: a call that its transport does not have, and in auto, a
// gateway that is unavailable. Only the call that failed is sent again, so
// that nothing is done twice.
type switcher struct {
	primary backend.Backend
	auto    bool
	http    func() (backend.Backend, error)
	note    func(format string, args ...any)
	warn    func(format string, args ...any)
	// switched is set after the gateway was unavailable in auto: the rest of
	// the command uses HTTPS.
	switched backend.Backend
}

func (s *switcher) current() backend.Backend {
	if s.switched != nil {
		return s.switched
	}
	return s.primary
}

func via[T any](s *switcher, call func(backend.Backend) (T, error)) (T, error) {
	if s.switched != nil {
		return call(s.switched)
	}
	v, err := call(s.primary)
	unavailable := s.auto && status.Code(err) == codes.Unavailable
	if err == nil || !unavailable && !errors.Is(err, backend.ErrNotAvailable) {
		return v, err
	}
	h, herr := s.http()
	if herr != nil {
		return v, fmt.Errorf("%w, and https cannot be used: %w", err, herr)
	}
	if unavailable {
		s.warn("the gateway is unavailable (%v): continuing over https", status.Convert(err).Message())
		s.switched = h
	} else {
		s.note("%v: using https for this call", err)
	}
	return call(h)
}

func run(s *switcher, call func(backend.Backend) error) error {
	_, err := via(s, func(b backend.Backend) (struct{}, error) { return struct{}{}, call(b) })
	return err
}

func (s *switcher) Transport() string          { return s.current().Transport() }
func (s *switcher) Files() backend.Files       { return sFiles{s} }
func (s *switcher) Shares() backend.Shares     { return sShares{s} }
func (s *switcher) Links() backend.Links       { return sLinks{s} }
func (s *switcher) Spaces() backend.Spaces     { return sSpaces{s} }
func (s *switcher) Trash() backend.Trash       { return sTrash{s} }
func (s *switcher) Versions() backend.Versions { return sVersions{s} }
func (s *switcher) Locks() backend.Locks       { return sLocks{s} }
func (s *switcher) OCM() backend.OCM           { return sOCM{s} }
func (s *switcher) Tokens() backend.Tokens     { return sTokens{s} }
func (s *switcher) Users() backend.Users       { return sUsers{s} }
func (s *switcher) Locator() backend.Locator   { return sLocator{s} }

func (s *switcher) ServerTime(ctx context.Context) (time.Time, error) {
	return via(s, func(b backend.Backend) (time.Time, error) { return b.ServerTime(ctx) })
}

// The forwarders of the areas follow.

type sFiles struct{ s *switcher }

func (a sFiles) Stat(ctx context.Context, ref *provider.Reference) (*provider.ResourceInfo, error) {
	return via(a.s, func(b backend.Backend) (*provider.ResourceInfo, error) { return b.Files().Stat(ctx, ref) })
}

func (a sFiles) List(ctx context.Context, ref *provider.Reference) ([]*provider.ResourceInfo, error) {
	return via(a.s, func(b backend.Backend) ([]*provider.ResourceInfo, error) { return b.Files().List(ctx, ref) })
}

func (a sFiles) Walk(ctx context.Context, ref *provider.Reference, fn func(*provider.ResourceInfo) error) error {
	return run(a.s, func(b backend.Backend) error { return b.Files().Walk(ctx, ref, fn) })
}

func (a sFiles) Mkdir(ctx context.Context, ref *provider.Reference, parents bool) error {
	return run(a.s, func(b backend.Backend) error { return b.Files().Mkdir(ctx, ref, parents) })
}

func (a sFiles) Move(ctx context.Context, src, dst *provider.Reference, overwrite bool) error {
	return run(a.s, func(b backend.Backend) error { return b.Files().Move(ctx, src, dst, overwrite) })
}

func (a sFiles) Copy(ctx context.Context, src, dst *provider.Reference, overwrite bool) error {
	return run(a.s, func(b backend.Backend) error { return b.Files().Copy(ctx, src, dst, overwrite) })
}

func (a sFiles) Remove(ctx context.Context, ref *provider.Reference) error {
	return run(a.s, func(b backend.Backend) error { return b.Files().Remove(ctx, ref) })
}

func (a sFiles) Touch(ctx context.Context, ref *provider.Reference) error {
	return run(a.s, func(b backend.Backend) error { return b.Files().Touch(ctx, ref) })
}

func (a sFiles) ResolveSpace(ctx context.Context, alias string) (*provider.Reference, error) {
	return via(a.s, func(b backend.Backend) (*provider.Reference, error) { return b.Files().ResolveSpace(ctx, alias) })
}

type sShares struct{ s *switcher }

func (a sShares) List(ctx context.Context, resource *provider.ResourceId) ([]*collaboration.Share, error) {
	return via(a.s, func(b backend.Backend) ([]*collaboration.Share, error) { return b.Shares().List(ctx, resource) })
}

func (a sShares) Get(ctx context.Context, id string) (*collaboration.Share, error) {
	return via(a.s, func(b backend.Backend) (*collaboration.Share, error) { return b.Shares().Get(ctx, id) })
}

func (a sShares) Create(ctx context.Context, req *collaboration.CreateShareRequest) (*collaboration.Share, error) {
	return via(a.s, func(b backend.Backend) (*collaboration.Share, error) { return b.Shares().Create(ctx, req) })
}

func (a sShares) Update(ctx context.Context, req *collaboration.UpdateShareRequest) (*collaboration.Share, error) {
	return via(a.s, func(b backend.Backend) (*collaboration.Share, error) { return b.Shares().Update(ctx, req) })
}

func (a sShares) Remove(ctx context.Context, id string) error {
	return run(a.s, func(b backend.Backend) error { return b.Shares().Remove(ctx, id) })
}

func (a sShares) Received(ctx context.Context) ([]*collaboration.ReceivedShare, error) {
	return via(a.s, func(b backend.Backend) ([]*collaboration.ReceivedShare, error) { return b.Shares().Received(ctx) })
}

func (a sShares) UpdateReceived(ctx context.Context, id string, state collaboration.ShareState) (*collaboration.ReceivedShare, error) {
	return via(a.s, func(b backend.Backend) (*collaboration.ReceivedShare, error) {
		return b.Shares().UpdateReceived(ctx, id, state)
	})
}

func (a sShares) HideReceived(ctx context.Context, id string) error {
	return run(a.s, func(b backend.Backend) error { return b.Shares().HideReceived(ctx, id) })
}

type sLinks struct{ s *switcher }

func (a sLinks) List(ctx context.Context, resource *provider.ResourceId) ([]*link.PublicShare, error) {
	return via(a.s, func(b backend.Backend) ([]*link.PublicShare, error) { return b.Links().List(ctx, resource) })
}

func (a sLinks) Get(ctx context.Context, ref *link.PublicShareReference) (*link.PublicShare, error) {
	return via(a.s, func(b backend.Backend) (*link.PublicShare, error) { return b.Links().Get(ctx, ref) })
}

func (a sLinks) Create(ctx context.Context, req *link.CreatePublicShareRequest) (*link.PublicShare, error) {
	return via(a.s, func(b backend.Backend) (*link.PublicShare, error) { return b.Links().Create(ctx, req) })
}

func (a sLinks) Update(ctx context.Context, req *link.UpdatePublicShareRequest) (*link.PublicShare, error) {
	return via(a.s, func(b backend.Backend) (*link.PublicShare, error) { return b.Links().Update(ctx, req) })
}

func (a sLinks) Remove(ctx context.Context, ref *link.PublicShareReference) error {
	return run(a.s, func(b backend.Backend) error { return b.Links().Remove(ctx, ref) })
}

type sSpaces struct{ s *switcher }

func (a sSpaces) List(ctx context.Context) ([]*provider.StorageSpace, error) {
	return via(a.s, func(b backend.Backend) ([]*provider.StorageSpace, error) { return b.Spaces().List(ctx) })
}

func (a sSpaces) Update(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.StorageSpace, error) {
	return via(a.s, func(b backend.Backend) (*provider.StorageSpace, error) { return b.Spaces().Update(ctx, req) })
}

func (a sSpaces) Quota(ctx context.Context, ref *provider.Reference) (*backend.Quota, error) {
	return via(a.s, func(b backend.Backend) (*backend.Quota, error) { return b.Spaces().Quota(ctx, ref) })
}

type sTrash struct{ s *switcher }

func (a sTrash) List(ctx context.Context, ref *provider.Reference, from, to time.Time) ([]*provider.RecycleItem, error) {
	return via(a.s, func(b backend.Backend) ([]*provider.RecycleItem, error) { return b.Trash().List(ctx, ref, from, to) })
}

func (a sTrash) Restore(ctx context.Context, ref *provider.Reference, key string) error {
	return run(a.s, func(b backend.Backend) error { return b.Trash().Restore(ctx, ref, key) })
}

func (a sTrash) Purge(ctx context.Context, ref *provider.Reference, key string) error {
	return run(a.s, func(b backend.Backend) error { return b.Trash().Purge(ctx, ref, key) })
}

type sVersions struct{ s *switcher }

func (a sVersions) List(ctx context.Context, ref *provider.Reference) ([]*provider.FileVersion, error) {
	return via(a.s, func(b backend.Backend) ([]*provider.FileVersion, error) { return b.Versions().List(ctx, ref) })
}

func (a sVersions) Restore(ctx context.Context, ref *provider.Reference, key string) error {
	return run(a.s, func(b backend.Backend) error { return b.Versions().Restore(ctx, ref, key) })
}

type sLocks struct{ s *switcher }

func (a sLocks) Get(ctx context.Context, ref *provider.Reference) (*provider.Lock, error) {
	return via(a.s, func(b backend.Backend) (*provider.Lock, error) { return b.Locks().Get(ctx, ref) })
}

func (a sLocks) Set(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	return run(a.s, func(b backend.Backend) error { return b.Locks().Set(ctx, ref, lock) })
}

func (a sLocks) Refresh(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	return run(a.s, func(b backend.Backend) error { return b.Locks().Refresh(ctx, ref, lock) })
}

func (a sLocks) Unlock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	return run(a.s, func(b backend.Backend) error { return b.Locks().Unlock(ctx, ref, lock) })
}

type sOCM struct{ s *switcher }

func (a sOCM) CreateInvite(ctx context.Context, description string) (*invitepb.InviteToken, error) {
	return via(a.s, func(b backend.Backend) (*invitepb.InviteToken, error) { return b.OCM().CreateInvite(ctx, description) })
}

func (a sOCM) Invites(ctx context.Context) ([]*invitepb.InviteToken, error) {
	return via(a.s, func(b backend.Backend) ([]*invitepb.InviteToken, error) { return b.OCM().Invites(ctx) })
}

func (a sOCM) AcceptInvite(ctx context.Context, token, domain string) error {
	return run(a.s, func(b backend.Backend) error { return b.OCM().AcceptInvite(ctx, token, domain) })
}

func (a sOCM) Contacts(ctx context.Context, filter string) ([]*userpb.User, error) {
	return via(a.s, func(b backend.Backend) ([]*userpb.User, error) { return b.OCM().Contacts(ctx, filter) })
}

func (a sOCM) RemoveContact(ctx context.Context, id *userpb.UserId) error {
	return run(a.s, func(b backend.Backend) error { return b.OCM().RemoveContact(ctx, id) })
}

func (a sOCM) Providers(ctx context.Context) ([]*ocmprovider.ProviderInfo, error) {
	return via(a.s, func(b backend.Backend) ([]*ocmprovider.ProviderInfo, error) { return b.OCM().Providers(ctx) })
}

func (a sOCM) CreateShare(ctx context.Context, ref *provider.Reference, grantee *userpb.UserId, methods []*ocm.AccessMethod) (*ocm.Share, error) {
	return via(a.s, func(b backend.Backend) (*ocm.Share, error) { return b.OCM().CreateShare(ctx, ref, grantee, methods) })
}

func (a sOCM) Shares(ctx context.Context, resource *provider.ResourceId) ([]*ocm.Share, error) {
	return via(a.s, func(b backend.Backend) ([]*ocm.Share, error) { return b.OCM().Shares(ctx, resource) })
}

func (a sOCM) GetShare(ctx context.Context, id string) (*ocm.Share, error) {
	return via(a.s, func(b backend.Backend) (*ocm.Share, error) { return b.OCM().GetShare(ctx, id) })
}

func (a sOCM) UpdateShare(ctx context.Context, req *ocm.UpdateOCMShareRequest) error {
	return run(a.s, func(b backend.Backend) error { return b.OCM().UpdateShare(ctx, req) })
}

func (a sOCM) RemoveShare(ctx context.Context, id string) error {
	return run(a.s, func(b backend.Backend) error { return b.OCM().RemoveShare(ctx, id) })
}

func (a sOCM) Received(ctx context.Context) ([]*ocm.ReceivedShare, error) {
	return via(a.s, func(b backend.Backend) ([]*ocm.ReceivedShare, error) { return b.OCM().Received(ctx) })
}

func (a sOCM) GetReceived(ctx context.Context, id string) (*ocm.ReceivedShare, error) {
	return via(a.s, func(b backend.Backend) (*ocm.ReceivedShare, error) { return b.OCM().GetReceived(ctx, id) })
}

func (a sOCM) UpdateReceived(ctx context.Context, id string, state ocm.ShareState) error {
	return run(a.s, func(b backend.Backend) error { return b.OCM().UpdateReceived(ctx, id, state) })
}

type sTokens struct{ s *switcher }

func (a sTokens) List(ctx context.Context) ([]*authapp.AppPassword, error) {
	return via(a.s, func(b backend.Backend) ([]*authapp.AppPassword, error) { return b.Tokens().List(ctx) })
}

func (a sTokens) Create(ctx context.Context, req *authapp.GenerateAppPasswordRequest) (*authapp.AppPassword, error) {
	return via(a.s, func(b backend.Backend) (*authapp.AppPassword, error) { return b.Tokens().Create(ctx, req) })
}

func (a sTokens) Revoke(ctx context.Context, id string) error {
	return run(a.s, func(b backend.Backend) error { return b.Tokens().Revoke(ctx, id) })
}

type sUsers struct{ s *switcher }

func (a sUsers) Me(ctx context.Context) (*userpb.User, error) {
	return via(a.s, func(b backend.Backend) (*userpb.User, error) { return b.Users().Me(ctx) })
}

func (a sUsers) ByClaim(ctx context.Context, claim, value string) (*userpb.User, error) {
	return via(a.s, func(b backend.Backend) (*userpb.User, error) { return b.Users().ByClaim(ctx, claim, value) })
}

type sLocator struct{ s *switcher }

func (a sLocator) DownloadTarget(ctx context.Context, ref *provider.Reference) (*backend.Target, error) {
	return via(a.s, func(b backend.Backend) (*backend.Target, error) { return b.Locator().DownloadTarget(ctx, ref) })
}

func (a sLocator) UploadTarget(ctx context.Context, ref *provider.Reference, size int64, ifMatch string) (*backend.Target, error) {
	return via(a.s, func(b backend.Backend) (*backend.Target, error) {
		return b.Locator().UploadTarget(ctx, ref, size, ifMatch)
	})
}

func (a sLocator) OpenInApp(ctx context.Context, ref *provider.Reference, opts backend.AppOptions) (*appprovider.OpenInAppURL, error) {
	return via(a.s, func(b backend.Backend) (*appprovider.OpenInAppURL, error) {
		return b.Locator().OpenInApp(ctx, ref, opts)
	})
}
