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

// Package clitest has a backend and a data plane in memory, so that the
// commands of the CLI can be tested without a server:
//
//	b := clitest.New("alice")
//	rt.UseBackend(b, b.Data())
//
// It keeps files, trash, versions, locks, shares, links and app tokens. The
// other calls answer that they are not available over the transport "fake".
package clitest

import (
	"bytes"
	"context"
	"io"
	"maps"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	authapp "github.com/cs3org/go-cs3apis/cs3/auth/applications/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
)

// Transport is the name of the transport of the fake.
const Transport = "fake"

// Backend implements backend.Backend in memory. It is safe for concurrent
// use.
type Backend struct {
	mu     sync.Mutex
	me     *userpb.User
	now    time.Time
	next   int
	files  map[string]*file
	trash  []*trashed
	locks  map[string]*provider.Lock
	shares []*collaboration.Share
	recvd  []*collaboration.ReceivedShare
	links  []*link.PublicShare
	tokens []*authapp.AppPassword
}

type file struct {
	info     *provider.ResourceInfo
	data     []byte
	versions []*provider.FileVersion
	old      map[string][]byte
}

type trashed struct {
	item  *provider.RecycleItem
	files map[string]*file
}

var _ backend.Backend = (*Backend)(nil)

// New returns a backend with an empty root folder, signed in as user.
func New(user string) *Backend {
	b := &Backend{
		me:    &userpb.User{Id: &userpb.UserId{Idp: "fake", OpaqueId: user, Type: userpb.UserType_USER_TYPE_PRIMARY}, Username: user},
		now:   time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC),
		files: map[string]*file{},
		locks: map[string]*provider.Lock{},
	}
	b.files["/"] = &file{info: b.newInfo("/", provider.ResourceType_RESOURCE_TYPE_CONTAINER)}
	return b
}

// Data returns the data plane on the files of b.
func (b *Backend) Data() *Data { return &Data{b: b} }

// AddFile creates the file p with content, and its missing parents.
func (b *Backend) AddFile(p string, content []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.mkdirAll(path.Dir(p))
	b.write(p, content)
}

// Content returns the content of the file p.
func (b *Backend) Content(p string) ([]byte, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	f, ok := b.files[path.Clean(p)]
	if !ok || f.info.Type != provider.ResourceType_RESOURCE_TYPE_FILE {
		return nil, false
	}
	return bytes.Clone(f.data), true
}

// AddReceived adds a share that the caller received.
func (b *Backend) AddReceived(rs *collaboration.ReceivedShare) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.recvd = append(b.recvd, rs)
}

func (b *Backend) id() string {
	b.next++
	return strconv.Itoa(b.next)
}

func (b *Backend) ts() *types.Timestamp {
	b.now = b.now.Add(time.Second)
	return &types.Timestamp{Seconds: uint64(b.now.Unix())}
}

func (b *Backend) newInfo(p string, t provider.ResourceType) *provider.ResourceInfo {
	id := b.id()
	return &provider.ResourceInfo{
		Type:  t,
		Id:    &provider.ResourceId{StorageId: Transport, OpaqueId: id},
		Path:  p,
		Owner: b.me.Id,
		Mtime: b.ts(),
		Etag:  id,
	}
}

func notFound(p string) error {
	return &backend.StatusError{Code: rpc.Code_CODE_NOT_FOUND, Msg: p + " not found"}
}

func notAvailable(what string) error { return backend.NotAvailable(Transport, what) }

// lookup returns the file of ref, which has a path or a resource id.
func (b *Backend) lookup(ref *provider.Reference) (string, *file, error) {
	if ref.GetResourceId() != nil {
		for p, f := range b.files {
			if f.info.Id.OpaqueId == ref.ResourceId.OpaqueId {
				return p, f, nil
			}
		}
		return "", nil, notFound(ref.ResourceId.OpaqueId)
	}
	p := path.Clean("/" + ref.GetPath())
	f, ok := b.files[p]
	if !ok {
		return p, nil, notFound(p)
	}
	return p, f, nil
}

func (b *Backend) children(dir string) []string {
	var out []string
	for p := range b.files {
		if p != "/" && path.Dir(p) == dir {
			out = append(out, p)
		}
	}
	slices.Sort(out)
	return out
}

// subtree returns p and all that is below it.
func (b *Backend) subtree(p string) []string {
	var out []string
	for q := range b.files {
		if q == p || strings.HasPrefix(q, strings.TrimSuffix(p, "/")+"/") {
			out = append(out, q)
		}
	}
	return out
}

func (b *Backend) mkdirAll(p string) {
	if _, ok := b.files[p]; ok || p == "/" {
		return
	}
	b.mkdirAll(path.Dir(p))
	b.files[p] = &file{info: b.newInfo(p, provider.ResourceType_RESOURCE_TYPE_CONTAINER)}
}

// write stores content in p, and keeps the previous content as a version.
func (b *Backend) write(p string, content []byte) {
	f, ok := b.files[p]
	if !ok {
		f = &file{info: b.newInfo(p, provider.ResourceType_RESOURCE_TYPE_FILE), old: map[string][]byte{}}
		b.files[p] = f
	} else {
		key := b.id()
		f.versions = append(f.versions, &provider.FileVersion{Key: key, Size: f.info.Size, Mtime: f.info.Mtime.GetSeconds(), Etag: f.info.Etag})
		f.old[key] = f.data
		f.info.Mtime, f.info.Etag = b.ts(), b.id()
	}
	f.data = bytes.Clone(content)
	f.info.Size = uint64(len(content))
}

func (b *Backend) Transport() string          { return Transport }
func (b *Backend) Files() backend.Files       { return files{b} }
func (b *Backend) Shares() backend.Shares     { return shares{b} }
func (b *Backend) Links() backend.Links       { return links{b} }
func (b *Backend) Spaces() backend.Spaces     { return spaces{} }
func (b *Backend) Trash() backend.Trash       { return trash{b} }
func (b *Backend) Versions() backend.Versions { return versions{b} }
func (b *Backend) Locks() backend.Locks       { return locks{b} }
func (b *Backend) OCM() backend.OCM           { return ocmArea{} }
func (b *Backend) Tokens() backend.Tokens     { return tokens{b} }
func (b *Backend) Users() backend.Users       { return users{b} }
func (b *Backend) Locator() backend.Locator   { return locator{} }

func (b *Backend) ServerTime(ctx context.Context) (time.Time, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.now, nil
}

type files struct{ b *Backend }

func (f files) Stat(ctx context.Context, ref *provider.Reference) (*provider.ResourceInfo, error) {
	f.b.mu.Lock()
	defer f.b.mu.Unlock()
	_, fl, err := f.b.lookup(ref)
	if err != nil {
		return nil, err
	}
	return fl.info, nil
}

func (f files) List(ctx context.Context, ref *provider.Reference) ([]*provider.ResourceInfo, error) {
	f.b.mu.Lock()
	defer f.b.mu.Unlock()
	p, fl, err := f.b.lookup(ref)
	if err != nil {
		return nil, err
	}
	if fl.info.Type != provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		return []*provider.ResourceInfo{fl.info}, nil
	}
	var out []*provider.ResourceInfo
	for _, c := range f.b.children(p) {
		out = append(out, f.b.files[c].info)
	}
	return out, nil
}

func (f files) Walk(ctx context.Context, ref *provider.Reference, fn func(*provider.ResourceInfo) error) error {
	info, err := f.Stat(ctx, ref)
	if err != nil {
		return err
	}
	if err := fn(info); err != nil || info.Type != provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		return err
	}
	children, err := f.List(ctx, ref)
	if err != nil {
		return err
	}
	for _, c := range children {
		if err := f.Walk(ctx, &provider.Reference{Path: c.Path}, fn); err != nil {
			return err
		}
	}
	return nil
}

func (f files) Mkdir(ctx context.Context, ref *provider.Reference, parents bool) error {
	f.b.mu.Lock()
	defer f.b.mu.Unlock()
	p, existing, _ := f.b.lookup(ref)
	switch _, parentErr := f.b.files[path.Dir(p)]; {
	case existing != nil && parents:
		return nil
	case existing != nil:
		return &backend.StatusError{Code: rpc.Code_CODE_ALREADY_EXISTS, Msg: p + " exists"}
	case !parentErr && !parents:
		return notFound(path.Dir(p))
	}
	f.b.mkdirAll(p)
	return nil
}

func (f files) Move(ctx context.Context, src, dst *provider.Reference, overwrite bool) error {
	return f.moveOrCopy(src, dst, overwrite, true)
}

func (f files) Copy(ctx context.Context, src, dst *provider.Reference, overwrite bool) error {
	return f.moveOrCopy(src, dst, overwrite, false)
}

func (f files) moveOrCopy(src, dst *provider.Reference, overwrite, move bool) error {
	f.b.mu.Lock()
	defer f.b.mu.Unlock()
	sp, _, err := f.b.lookup(src)
	if err != nil {
		return err
	}
	dp, existing, _ := f.b.lookup(dst)
	if existing != nil && !overwrite {
		return &backend.StatusError{Code: rpc.Code_CODE_ALREADY_EXISTS, Msg: dp + " exists"}
	}
	if _, ok := f.b.files[path.Dir(dp)]; !ok {
		return notFound(path.Dir(dp))
	}
	for _, q := range f.b.subtree(dp) {
		delete(f.b.files, q)
	}
	for _, q := range f.b.subtree(sp) {
		old := f.b.files[q]
		np := dp + strings.TrimPrefix(q, sp)
		nf := &file{info: f.b.newInfo(np, old.info.Type), data: bytes.Clone(old.data), old: map[string][]byte{}}
		nf.info.Size = old.info.Size
		if move {
			nf = old
			nf.info.Path = np
			delete(f.b.files, q)
		}
		f.b.files[np] = nf
	}
	return nil
}

func (f files) Remove(ctx context.Context, ref *provider.Reference) error {
	f.b.mu.Lock()
	defer f.b.mu.Unlock()
	p, fl, err := f.b.lookup(ref)
	if err != nil {
		return err
	}
	t := &trashed{
		item:  &provider.RecycleItem{Type: fl.info.Type, Key: f.b.id(), Ref: &provider.Reference{Path: p}, Size: fl.info.Size, DeletionTime: f.b.ts()},
		files: map[string]*file{},
	}
	for _, q := range f.b.subtree(p) {
		t.files[q] = f.b.files[q]
		delete(f.b.files, q)
	}
	f.b.trash = append(f.b.trash, t)
	return nil
}

func (f files) Touch(ctx context.Context, ref *provider.Reference) error {
	f.b.mu.Lock()
	defer f.b.mu.Unlock()
	p, existing, _ := f.b.lookup(ref)
	if existing != nil {
		existing.info.Mtime = f.b.ts()
		return nil
	}
	if _, ok := f.b.files[path.Dir(p)]; !ok {
		return notFound(path.Dir(p))
	}
	f.b.write(p, nil)
	return nil
}

func (f files) ResolveSpace(ctx context.Context, alias string) (*provider.Reference, error) {
	if alias == "home" || alias == "personal" {
		return &provider.Reference{Path: "/"}, nil
	}
	return nil, notFound("space " + alias)
}

type trash struct{ b *Backend }

func (t trash) List(ctx context.Context, ref *provider.Reference, from, to time.Time) ([]*provider.RecycleItem, error) {
	t.b.mu.Lock()
	defer t.b.mu.Unlock()
	var out []*provider.RecycleItem
	for _, it := range t.b.trash {
		at := time.Unix(int64(it.item.DeletionTime.Seconds), 0)
		if (from.IsZero() || !at.Before(from)) && (to.IsZero() || !at.After(to)) {
			out = append(out, it.item)
		}
	}
	return out, nil
}

func (t trash) find(key string) (int, error) {
	i := slices.IndexFunc(t.b.trash, func(it *trashed) bool { return it.item.Key == key })
	if i < 0 {
		return 0, notFound("trash item " + key)
	}
	return i, nil
}

func (t trash) Restore(ctx context.Context, ref *provider.Reference, key string) error {
	t.b.mu.Lock()
	defer t.b.mu.Unlock()
	i, err := t.find(key)
	if err != nil {
		return err
	}
	it := t.b.trash[i]
	if _, ok := t.b.files[it.item.Ref.Path]; ok {
		return &backend.StatusError{Code: rpc.Code_CODE_ALREADY_EXISTS, Msg: it.item.Ref.Path + " exists"}
	}
	t.b.mkdirAll(path.Dir(it.item.Ref.Path))
	maps.Copy(t.b.files, it.files)
	t.b.trash = slices.Delete(t.b.trash, i, i+1)
	return nil
}

func (t trash) Purge(ctx context.Context, ref *provider.Reference, key string) error {
	t.b.mu.Lock()
	defer t.b.mu.Unlock()
	i, err := t.find(key)
	if err != nil {
		return err
	}
	t.b.trash = slices.Delete(t.b.trash, i, i+1)
	return nil
}

type versions struct{ b *Backend }

func (v versions) List(ctx context.Context, ref *provider.Reference) ([]*provider.FileVersion, error) {
	v.b.mu.Lock()
	defer v.b.mu.Unlock()
	_, f, err := v.b.lookup(ref)
	if err != nil {
		return nil, err
	}
	return f.versions, nil
}

func (v versions) Restore(ctx context.Context, ref *provider.Reference, key string) error {
	v.b.mu.Lock()
	defer v.b.mu.Unlock()
	p, f, err := v.b.lookup(ref)
	if err != nil {
		return err
	}
	data, ok := f.old[key]
	if !ok {
		return notFound("version " + key)
	}
	v.b.write(p, data)
	return nil
}

type locks struct{ b *Backend }

func (l locks) Get(ctx context.Context, ref *provider.Reference) (*provider.Lock, error) {
	l.b.mu.Lock()
	defer l.b.mu.Unlock()
	p, _, err := l.b.lookup(ref)
	if err != nil {
		return nil, err
	}
	lock, ok := l.b.locks[p]
	if !ok {
		return nil, notFound("lock of " + p)
	}
	return lock, nil
}

func (l locks) Set(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	l.b.mu.Lock()
	defer l.b.mu.Unlock()
	p, _, err := l.b.lookup(ref)
	if err != nil {
		return err
	}
	if _, ok := l.b.locks[p]; ok {
		return &backend.StatusError{Code: rpc.Code_CODE_FAILED_PRECONDITION, Msg: p + " is locked"}
	}
	l.b.locks[p] = lock
	return nil
}

func (l locks) Refresh(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	l.b.mu.Lock()
	defer l.b.mu.Unlock()
	p, _, err := l.b.lookup(ref)
	if err != nil {
		return err
	}
	if old, ok := l.b.locks[p]; !ok || old.LockId != lock.LockId {
		return &backend.StatusError{Code: rpc.Code_CODE_FAILED_PRECONDITION, Msg: "no lock " + lock.LockId}
	}
	l.b.locks[p] = lock
	return nil
}

func (l locks) Unlock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	l.b.mu.Lock()
	defer l.b.mu.Unlock()
	p, _, err := l.b.lookup(ref)
	if err != nil {
		return err
	}
	if old, ok := l.b.locks[p]; !ok || old.LockId != lock.GetLockId() {
		return &backend.StatusError{Code: rpc.Code_CODE_FAILED_PRECONDITION, Msg: "no lock " + lock.GetLockId()}
	}
	delete(l.b.locks, p)
	return nil
}

type shares struct{ b *Backend }

func (s shares) List(ctx context.Context, resource *provider.ResourceId) ([]*collaboration.Share, error) {
	s.b.mu.Lock()
	defer s.b.mu.Unlock()
	var out []*collaboration.Share
	for _, sh := range s.b.shares {
		if resource == nil || sh.ResourceId.OpaqueId == resource.OpaqueId {
			out = append(out, sh)
		}
	}
	return out, nil
}

func (s shares) index(id string) (int, error) {
	i := slices.IndexFunc(s.b.shares, func(sh *collaboration.Share) bool { return sh.Id.OpaqueId == id })
	if i < 0 {
		return 0, notFound("share " + id)
	}
	return i, nil
}

func (s shares) Get(ctx context.Context, id string) (*collaboration.Share, error) {
	s.b.mu.Lock()
	defer s.b.mu.Unlock()
	i, err := s.index(id)
	if err != nil {
		return nil, err
	}
	return s.b.shares[i], nil
}

func (s shares) Create(ctx context.Context, req *collaboration.CreateShareRequest) (*collaboration.Share, error) {
	s.b.mu.Lock()
	defer s.b.mu.Unlock()
	now := s.b.ts()
	sh := &collaboration.Share{
		Id:          &collaboration.ShareId{OpaqueId: s.b.id()},
		ResourceId:  req.ResourceInfo.Id,
		Permissions: req.Grant.Permissions,
		Grantee:     req.Grant.Grantee,
		Owner:       s.b.me.Id,
		Creator:     s.b.me.Id,
		Ctime:       now,
		Mtime:       now,
	}
	s.b.shares = append(s.b.shares, sh)
	return sh, nil
}

func (s shares) Update(ctx context.Context, req *collaboration.UpdateShareRequest) (*collaboration.Share, error) {
	s.b.mu.Lock()
	defer s.b.mu.Unlock()
	i, err := s.index(req.Ref.GetId().GetOpaqueId())
	if err != nil {
		return nil, err
	}
	sh := s.b.shares[i]
	if p := req.GetField().GetPermissions(); p != nil {
		sh.Permissions = p
	}
	sh.Mtime = s.b.ts()
	return sh, nil
}

func (s shares) Remove(ctx context.Context, id string) error {
	s.b.mu.Lock()
	defer s.b.mu.Unlock()
	i, err := s.index(id)
	if err != nil {
		return err
	}
	s.b.shares = slices.Delete(s.b.shares, i, i+1)
	return nil
}

func (s shares) Received(ctx context.Context) ([]*collaboration.ReceivedShare, error) {
	s.b.mu.Lock()
	defer s.b.mu.Unlock()
	return slices.Clone(s.b.recvd), nil
}

func (s shares) received(id string) (*collaboration.ReceivedShare, error) {
	for _, rs := range s.b.recvd {
		if rs.Share.Id.OpaqueId == id {
			return rs, nil
		}
	}
	return nil, notFound("received share " + id)
}

func (s shares) UpdateReceived(ctx context.Context, id string, state collaboration.ShareState) (*collaboration.ReceivedShare, error) {
	s.b.mu.Lock()
	defer s.b.mu.Unlock()
	rs, err := s.received(id)
	if err != nil {
		return nil, err
	}
	rs.State = state
	return rs, nil
}

func (s shares) HideReceived(ctx context.Context, id string) error {
	s.b.mu.Lock()
	defer s.b.mu.Unlock()
	rs, err := s.received(id)
	if err != nil {
		return err
	}
	rs.Hidden = true
	return nil
}

type links struct{ b *Backend }

func (l links) List(ctx context.Context, resource *provider.ResourceId) ([]*link.PublicShare, error) {
	l.b.mu.Lock()
	defer l.b.mu.Unlock()
	var out []*link.PublicShare
	for _, s := range l.b.links {
		if resource == nil || s.ResourceId.OpaqueId == resource.OpaqueId {
			out = append(out, s)
		}
	}
	return out, nil
}

func (l links) index(ref *link.PublicShareReference) (int, error) {
	i := slices.IndexFunc(l.b.links, func(s *link.PublicShare) bool {
		if t := ref.GetToken(); t != "" {
			return s.Token == t
		}
		return s.Id.OpaqueId == ref.GetId().GetOpaqueId()
	})
	if i < 0 {
		return 0, notFound("link")
	}
	return i, nil
}

func (l links) Get(ctx context.Context, ref *link.PublicShareReference) (*link.PublicShare, error) {
	l.b.mu.Lock()
	defer l.b.mu.Unlock()
	i, err := l.index(ref)
	if err != nil {
		return nil, err
	}
	return l.b.links[i], nil
}

func (l links) Create(ctx context.Context, req *link.CreatePublicShareRequest) (*link.PublicShare, error) {
	l.b.mu.Lock()
	defer l.b.mu.Unlock()
	now := l.b.ts()
	id := l.b.id()
	s := &link.PublicShare{
		Id:          &link.PublicShareId{OpaqueId: id},
		Token:       "token" + id,
		ResourceId:  req.ResourceInfo.Id,
		Permissions: req.Grant.GetPermissions(),
		Owner:       l.b.me.Id,
		Creator:     l.b.me.Id,
		Ctime:       now,
		Mtime:       now,
		Description: req.Description,
	}
	l.b.links = append(l.b.links, s)
	return s, nil
}

func (l links) Update(ctx context.Context, req *link.UpdatePublicShareRequest) (*link.PublicShare, error) {
	l.b.mu.Lock()
	defer l.b.mu.Unlock()
	i, err := l.index(req.Ref)
	if err != nil {
		return nil, err
	}
	s := l.b.links[i]
	switch req.Update.GetType() {
	case link.UpdatePublicShareRequest_Update_TYPE_PERMISSIONS:
		s.Permissions = req.Update.GetGrant().GetPermissions()
	case link.UpdatePublicShareRequest_Update_TYPE_DESCRIPTION:
		s.Description = req.Update.GetDescription()
	}
	s.Mtime = l.b.ts()
	return s, nil
}

func (l links) Remove(ctx context.Context, ref *link.PublicShareReference) error {
	l.b.mu.Lock()
	defer l.b.mu.Unlock()
	i, err := l.index(ref)
	if err != nil {
		return err
	}
	l.b.links = slices.Delete(l.b.links, i, i+1)
	return nil
}

type tokens struct{ b *Backend }

func (t tokens) List(ctx context.Context) ([]*authapp.AppPassword, error) {
	t.b.mu.Lock()
	defer t.b.mu.Unlock()
	return slices.Clone(t.b.tokens), nil
}

func (t tokens) Create(ctx context.Context, req *authapp.GenerateAppPasswordRequest) (*authapp.AppPassword, error) {
	t.b.mu.Lock()
	defer t.b.mu.Unlock()
	pw := &authapp.AppPassword{
		Password:   "secret" + t.b.id(),
		TokenScope: req.TokenScope,
		Label:      req.Label,
		User:       t.b.me.Id,
		Expiration: req.Expiration,
		Ctime:      t.b.ts(),
	}
	t.b.tokens = append(t.b.tokens, pw)
	return pw, nil
}

func (t tokens) Revoke(ctx context.Context, id string) error {
	t.b.mu.Lock()
	defer t.b.mu.Unlock()
	i := slices.IndexFunc(t.b.tokens, func(pw *authapp.AppPassword) bool { return pw.Password == id })
	if i < 0 {
		return notFound("token " + id)
	}
	t.b.tokens = slices.Delete(t.b.tokens, i, i+1)
	return nil
}

type users struct{ b *Backend }

func (u users) Me(ctx context.Context) (*userpb.User, error) { return u.b.me, nil }

func (u users) ByClaim(ctx context.Context, claim, value string) (*userpb.User, error) {
	if claim == "username" && value == u.b.me.Username {
		return u.b.me, nil
	}
	return nil, notFound("user " + value)
}

type spaces struct{}

func (spaces) List(ctx context.Context) ([]*provider.StorageSpace, error) {
	return nil, notAvailable("the space list")
}

func (spaces) Quota(ctx context.Context, ref *provider.Reference) (*backend.Quota, error) {
	return nil, notAvailable("the quota")
}

type ocmArea struct{}

func (ocmArea) CreateInvite(ctx context.Context, description string) (*invitepb.InviteToken, error) {
	return nil, notAvailable("OCM")
}

func (ocmArea) Invites(ctx context.Context) ([]*invitepb.InviteToken, error) {
	return nil, notAvailable("OCM")
}

func (ocmArea) AcceptInvite(ctx context.Context, token, domain string) error {
	return notAvailable("OCM")
}

func (ocmArea) Contacts(ctx context.Context, filter string) ([]*userpb.User, error) {
	return nil, notAvailable("OCM")
}

func (ocmArea) RemoveContact(ctx context.Context, id *userpb.UserId) error {
	return notAvailable("OCM")
}

func (ocmArea) Providers(ctx context.Context) ([]*ocmprovider.ProviderInfo, error) {
	return nil, notAvailable("OCM")
}

func (ocmArea) CreateShare(ctx context.Context, ref *provider.Reference, grantee *userpb.UserId, methods []*ocm.AccessMethod) (*ocm.Share, error) {
	return nil, notAvailable("OCM")
}

func (ocmArea) Shares(ctx context.Context, resource *provider.ResourceId) ([]*ocm.Share, error) {
	return nil, notAvailable("OCM")
}

func (ocmArea) GetShare(ctx context.Context, id string) (*ocm.Share, error) {
	return nil, notAvailable("OCM")
}

func (ocmArea) UpdateShare(ctx context.Context, req *ocm.UpdateOCMShareRequest) error {
	return notAvailable("OCM")
}

func (ocmArea) RemoveShare(ctx context.Context, id string) error {
	return notAvailable("OCM")
}

func (ocmArea) Received(ctx context.Context) ([]*ocm.ReceivedShare, error) {
	return nil, notAvailable("OCM")
}

func (ocmArea) GetReceived(ctx context.Context, id string) (*ocm.ReceivedShare, error) {
	return nil, notAvailable("OCM")
}

func (ocmArea) UpdateReceived(ctx context.Context, id string, state ocm.ShareState) error {
	return notAvailable("OCM")
}

// locator has no targets: Data moves the bytes in memory.
type locator struct{}

func (locator) DownloadTarget(ctx context.Context, ref *provider.Reference) (*backend.Target, error) {
	return nil, notAvailable("a download target")
}

func (locator) UploadTarget(ctx context.Context, ref *provider.Reference, size int64, ifMatch string) (*backend.Target, error) {
	return nil, notAvailable("an upload target")
}

func (locator) OpenInApp(ctx context.Context, ref *provider.Reference, opts backend.AppOptions) (*appprovider.OpenInAppURL, error) {
	return nil, notAvailable("app open")
}

// Data implements backend.DataPlane on the files of a Backend.
type Data struct{ b *Backend }

var _ backend.DataPlane = (*Data)(nil)

func (d *Data) Download(ctx context.Context, ref *provider.Reference, offset int64) (io.ReadCloser, int64, error) {
	d.b.mu.Lock()
	defer d.b.mu.Unlock()
	p, f, err := d.b.lookup(ref)
	if err != nil {
		return nil, 0, err
	}
	if f.info.Type != provider.ResourceType_RESOURCE_TYPE_FILE {
		return nil, 0, &backend.StatusError{Code: rpc.Code_CODE_FAILED_PRECONDITION, Msg: p + " is a folder"}
	}
	data := f.data[min(offset, int64(len(f.data))):]
	return io.NopCloser(bytes.NewReader(bytes.Clone(data))), int64(len(data)), nil
}

func (d *Data) Upload(ctx context.Context, ref *provider.Reference, open func() (io.ReadCloser, error), size int64, checksum string) error {
	return d.put(ref, open, "")
}

func (d *Data) UploadIfUnchanged(ctx context.Context, ref *provider.Reference, open func() (io.ReadCloser, error), size int64, etag string) error {
	return d.put(ref, open, etag)
}

func (d *Data) UploadStream(ctx context.Context, ref *provider.Reference, r io.Reader, size int64) error {
	return d.put(ref, func() (io.ReadCloser, error) { return io.NopCloser(r), nil }, "")
}

func (d *Data) UploadResumable(ctx context.Context, ref *provider.Reference, open func() (io.ReadCloser, error), size int64, checksum string) error {
	return d.put(ref, open, "")
}

func (d *Data) put(ref *provider.Reference, open func() (io.ReadCloser, error), etag string) error {
	r, err := open()
	if err != nil {
		return err
	}
	defer r.Close()
	content, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	d.b.mu.Lock()
	defer d.b.mu.Unlock()
	p, f, _ := d.b.lookup(ref)
	if etag != "" && (f == nil || f.info.Etag != etag) {
		return &backend.StatusError{Code: rpc.Code_CODE_FAILED_PRECONDITION, Msg: "the etag of " + p + " changed"}
	}
	if _, ok := d.b.files[path.Dir(p)]; !ok {
		return notFound(path.Dir(p))
	}
	d.b.write(p, content)
	return nil
}

func (d *Data) Archive(ctx context.Context, refs []*provider.Reference, format string) (io.ReadCloser, error) {
	return nil, notAvailable("archive")
}

func (d *Data) OpenInApp(ctx context.Context, ref *provider.Reference, opts backend.AppOptions) (*appprovider.OpenInAppURL, error) {
	info, err := d.b.Files().Stat(ctx, ref)
	if err != nil {
		return nil, err
	}
	return &appprovider.OpenInAppURL{AppUrl: "https://app.example.org/open?id=" + info.Id.OpaqueId, Method: "GET"}, nil
}
