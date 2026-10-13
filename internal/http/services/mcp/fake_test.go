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

package mcp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	labels "github.com/cs3org/go-cs3apis/cs3/labels/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/datagateway"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/mime"
	"github.com/cs3org/reva/v3/pkg/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	home          = "/eos/user/e/einstein"
	project       = "/eos/project/c/cernbox"
	transferToken = "transfer-token"
)

var einstein = &userpb.User{Id: &userpb.UserId{OpaqueId: "einstein-id", Idp: "cern"}, Username: "einstein"}

type fakeFile struct {
	info    *provider.ResourceInfo
	content []byte
}

// fakeGateway is an in-memory storage behind a gateway, with a data server.
// Calls on a denied path fail as the auth interceptor fails them.
type fakeGateway struct {
	gateway.GatewayAPIClient

	mu       sync.Mutex
	files    map[string]*fakeFile
	trash    []*provider.RecycleItem
	trashed  map[string]*fakeFile
	versions map[string][]*provider.FileVersion
	old      map[string][]byte
	shares   []*collaboration.Share
	received []*collaboration.ReceivedShare
	links    []*link.PublicShare
	users    map[string]*userpb.User
	denied   []string
	n        int

	// What reached the gateway and the data server.
	methods    []string
	tokens     []string
	dataTokens []string

	data *httptest.Server
}

func newFakeGateway(t *testing.T) *fakeGateway {
	g := &fakeGateway{
		files:    map[string]*fakeFile{},
		trashed:  map[string]*fakeFile{},
		versions: map[string][]*provider.FileVersion{},
		old:      map[string][]byte{},
		users: map[string]*userpb.User{
			"einstein": einstein,
			"marie":    {Id: &userpb.UserId{OpaqueId: "marie-id", Idp: "cern"}, Username: "marie"},
		},
	}
	g.data = httptest.NewServer(http.HandlerFunc(g.serveData))
	t.Cleanup(g.data.Close)
	for _, d := range []string{"/eos", "/eos/user", "/eos/user/e", home, "/eos/project", "/eos/project/c", project} {
		g.mkdir(d)
	}
	return g
}

func (g *fakeGateway) nextID() int {
	g.n++
	return g.n
}

func (g *fakeGateway) mkdir(p string) *fakeFile {
	n := g.nextID()
	f := &fakeFile{info: &provider.ResourceInfo{
		Id:       &provider.ResourceId{StorageId: "eos", SpaceId: "space", OpaqueId: strconv.Itoa(n)},
		Path:     p,
		Type:     provider.ResourceType_RESOURCE_TYPE_CONTAINER,
		MimeType: mime.Detect(true, p),
		Etag:     fmt.Sprintf(`"%d"`, n),
		Mtime:    &types.Timestamp{Seconds: uint64(1760000000 + n)},
	}}
	g.files[p] = f
	return f
}

// put writes a file, keeping the previous content as a version.
func (g *fakeGateway) put(p string, content []byte) *fakeFile {
	n := g.nextID()
	if f, ok := g.files[p]; ok {
		key := strconv.Itoa(n)
		g.versions[p] = append(g.versions[p], &provider.FileVersion{Key: key, Size: f.info.Size, Mtime: f.info.Mtime.Seconds, Etag: f.info.Etag})
		g.old[p+"#"+key] = f.content
		f.content = content
		f.info.Size = uint64(len(content))
		f.info.Etag = fmt.Sprintf(`"%d"`, n)
		f.info.Mtime = &types.Timestamp{Seconds: uint64(1760000000 + n)}
		return f
	}
	f := &fakeFile{content: content, info: &provider.ResourceInfo{
		Id:       &provider.ResourceId{StorageId: "eos", SpaceId: "space", OpaqueId: strconv.Itoa(n)},
		Path:     p,
		Type:     provider.ResourceType_RESOURCE_TYPE_FILE,
		MimeType: mime.Detect(false, p),
		Size:     uint64(len(content)),
		Etag:     fmt.Sprintf(`"%d"`, n),
		Mtime:    &types.Timestamp{Seconds: uint64(1760000000 + n)},
	}}
	g.files[p] = f
	return f
}

// Put writes a file for a test.
func (g *fakeGateway) Put(p, content string) *provider.ResourceInfo {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.put(p, []byte(content)).info
}

// Mkdir creates a folder for a test.
func (g *fakeGateway) Mkdir(p string) *provider.ResourceInfo {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.mkdir(p).info
}

// Deny makes every call on a path under p fail as the auth interceptor does.
func (g *fakeGateway) Deny(p string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.denied = append(g.denied, p)
}

func (g *fakeGateway) Methods() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return slices.Clone(g.methods)
}

func (g *fakeGateway) Tokens() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return slices.Clone(g.tokens)
}

func (g *fakeGateway) DataTokens() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return slices.Clone(g.dataTokens)
}

// call records a gateway call with the token a gRPC client would send.
func (g *fakeGateway) call(ctx context.Context, method string) {
	g.methods = append(g.methods, method)
	md, _ := metadata.FromOutgoingContext(ctx)
	g.tokens = append(g.tokens, strings.Join(md.Get(appctx.TokenHeader), ","))
}

func (g *fakeGateway) deny(p string) error {
	for _, d := range g.denied {
		if p == d || strings.HasPrefix(p, d+"/") {
			return status.Error(codes.PermissionDenied, "auth: core access token is invalid")
		}
	}
	return nil
}

func (g *fakeGateway) lookup(ref *provider.Reference) (*fakeFile, string, error) {
	p := ref.GetPath()
	if id := ref.GetResourceId(); id != nil {
		p = ""
		for fp, f := range g.files {
			if f.info.Id.OpaqueId == id.OpaqueId {
				p = fp
			}
		}
	}
	if p != "" {
		if err := g.deny(p); err != nil {
			return nil, p, err
		}
	}
	return g.files[p], p, nil
}

func ok() *rpc.Status { return &rpc.Status{Code: rpc.Code_CODE_OK} }

func code(c rpc.Code) *rpc.Status { return &rpc.Status{Code: c, Message: c.String()} }

func (g *fakeGateway) Stat(ctx context.Context, req *provider.StatRequest, _ ...grpc.CallOption) (*provider.StatResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.call(ctx, "Stat")
	f, _, err := g.lookup(req.Ref)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return &provider.StatResponse{Status: code(rpc.Code_CODE_NOT_FOUND)}, nil
	}
	return &provider.StatResponse{Status: ok(), Info: f.info}, nil
}

func (g *fakeGateway) ListContainer(ctx context.Context, req *provider.ListContainerRequest, _ ...grpc.CallOption) (*provider.ListContainerResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.call(ctx, "ListContainer")
	f, p, err := g.lookup(req.Ref)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return &provider.ListContainerResponse{Status: code(rpc.Code_CODE_NOT_FOUND)}, nil
	}
	var infos []*provider.ResourceInfo
	for fp, c := range g.files {
		if path.Dir(fp) == p && fp != p {
			infos = append(infos, c.info)
		}
	}
	// In no particular order, as a storage does.
	slices.SortFunc(infos, func(a, b *provider.ResourceInfo) int { return strings.Compare(b.Path, a.Path) })
	return &provider.ListContainerResponse{Status: ok(), Infos: infos}, nil
}

func (g *fakeGateway) CreateContainer(ctx context.Context, req *provider.CreateContainerRequest, _ ...grpc.CallOption) (*provider.CreateContainerResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.call(ctx, "CreateContainer")
	f, p, err := g.lookup(req.Ref)
	if err != nil {
		return nil, err
	}
	if f != nil {
		return &provider.CreateContainerResponse{Status: code(rpc.Code_CODE_ALREADY_EXISTS)}, nil
	}
	if g.files[path.Dir(p)] == nil {
		return &provider.CreateContainerResponse{Status: code(rpc.Code_CODE_NOT_FOUND)}, nil
	}
	g.mkdir(p)
	return &provider.CreateContainerResponse{Status: ok()}, nil
}

func (g *fakeGateway) Delete(ctx context.Context, req *provider.DeleteRequest, _ ...grpc.CallOption) (*provider.DeleteResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.call(ctx, "Delete")
	f, p, err := g.lookup(req.Ref)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return &provider.DeleteResponse{Status: code(rpc.Code_CODE_NOT_FOUND)}, nil
	}
	key := strconv.Itoa(g.nextID())
	g.trash = append(g.trash, &provider.RecycleItem{
		Key: key, Ref: &provider.Reference{Path: p}, Type: f.info.Type, Size: f.info.Size,
		DeletionTime: &types.Timestamp{Seconds: uint64(1760000000 + g.n)},
	})
	g.trashed[key] = f
	delete(g.files, p)
	return &provider.DeleteResponse{Status: ok()}, nil
}

func (g *fakeGateway) Move(ctx context.Context, req *provider.MoveRequest, _ ...grpc.CallOption) (*provider.MoveResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.call(ctx, "Move")
	f, src, err := g.lookup(req.Source)
	if err != nil {
		return nil, err
	}
	dst := req.Destination.GetPath()
	if err := g.deny(dst); err != nil {
		return nil, err
	}
	if f == nil {
		return &provider.MoveResponse{Status: code(rpc.Code_CODE_NOT_FOUND)}, nil
	}
	delete(g.files, src)
	f.info.Path = dst
	g.files[dst] = f
	return &provider.MoveResponse{Status: ok()}, nil
}

func (g *fakeGateway) InitiateFileDownload(ctx context.Context, req *provider.InitiateFileDownloadRequest, _ ...grpc.CallOption) (*gateway.InitiateFileDownloadResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.call(ctx, "InitiateFileDownload")
	f, p, err := g.lookup(req.Ref)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return &gateway.InitiateFileDownloadResponse{Status: code(rpc.Code_CODE_NOT_FOUND)}, nil
	}
	return &gateway.InitiateFileDownloadResponse{Status: ok(), Protocols: []*gateway.FileDownloadProtocol{
		{Protocol: "spaces", DownloadEndpoint: "http://nowhere.invalid"},
		{Protocol: "simple", DownloadEndpoint: g.data.URL + "/data" + p, Token: transferToken},
	}}, nil
}

func (g *fakeGateway) InitiateFileUpload(ctx context.Context, req *provider.InitiateFileUploadRequest, _ ...grpc.CallOption) (*gateway.InitiateFileUploadResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.call(ctx, "InitiateFileUpload")
	_, p, err := g.lookup(req.Ref)
	if err != nil {
		return nil, err
	}
	if g.files[path.Dir(p)] == nil {
		return &gateway.InitiateFileUploadResponse{Status: code(rpc.Code_CODE_NOT_FOUND)}, nil
	}
	return &gateway.InitiateFileUploadResponse{Status: ok(), Protocols: []*gateway.FileUploadProtocol{
		{Protocol: "simple", UploadEndpoint: g.data.URL + "/data" + p, Token: transferToken},
	}}, nil
}

// serveData is the data gateway and the data provider in one.
func (g *fakeGateway) serveData(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	g.dataTokens = append(g.dataTokens, r.Header.Get(appctx.TokenHeader))
	g.mu.Unlock()
	if r.Header.Get(datagateway.TokenTransportHeader) != transferToken {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/data")
	switch r.Method {
	case http.MethodGet:
		g.mu.Lock()
		f := g.files[p]
		g.mu.Unlock()
		if f == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		http.ServeContent(w, r, p, time.Time{}, bytes.NewReader(f.content))
	case http.MethodPut:
		b, _ := io.ReadAll(r.Body)
		if r.Header.Get("Upload-Length") != strconv.Itoa(len(b)) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		g.mu.Lock()
		g.put(p, b)
		g.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}
}

func (g *fakeGateway) GetHome(ctx context.Context, req *provider.GetHomeRequest, _ ...grpc.CallOption) (*provider.GetHomeResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.call(ctx, "GetHome")
	return &provider.GetHomeResponse{Status: ok(), Path: home}, nil
}

func (g *fakeGateway) ListStorageSpaces(ctx context.Context, req *provider.ListStorageSpacesRequest, _ ...grpc.CallOption) (*provider.ListStorageSpacesResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.call(ctx, "ListStorageSpaces")
	return &provider.ListStorageSpacesResponse{Status: ok(), StorageSpaces: []*provider.StorageSpace{
		{Id: &provider.StorageSpaceId{OpaqueId: "home-space"}, Name: "Einstein", SpaceType: "home", RootInfo: g.files[home].info},
		{Id: &provider.StorageSpaceId{OpaqueId: "cernbox-space"}, Name: "cernbox", SpaceType: "project", RootInfo: g.files[project].info},
	}}, nil
}

func (g *fakeGateway) ListFileVersions(ctx context.Context, req *provider.ListFileVersionsRequest, _ ...grpc.CallOption) (*provider.ListFileVersionsResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.call(ctx, "ListFileVersions")
	f, p, err := g.lookup(req.Ref)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return &provider.ListFileVersionsResponse{Status: code(rpc.Code_CODE_NOT_FOUND)}, nil
	}
	return &provider.ListFileVersionsResponse{Status: ok(), Versions: g.versions[p]}, nil
}

func (g *fakeGateway) RestoreFileVersion(ctx context.Context, req *provider.RestoreFileVersionRequest, _ ...grpc.CallOption) (*provider.RestoreFileVersionResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.call(ctx, "RestoreFileVersion")
	f, p, err := g.lookup(req.Ref)
	if err != nil {
		return nil, err
	}
	content, found := g.old[p+"#"+req.Key]
	if f == nil || !found {
		return &provider.RestoreFileVersionResponse{Status: code(rpc.Code_CODE_NOT_FOUND)}, nil
	}
	g.put(p, content)
	return &provider.RestoreFileVersionResponse{Status: ok()}, nil
}

func (g *fakeGateway) ListRecycle(ctx context.Context, req *provider.ListRecycleRequest, _ ...grpc.CallOption) (*provider.ListRecycleResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.call(ctx, "ListRecycle")
	if _, _, err := g.lookup(req.Ref); err != nil {
		return nil, err
	}
	return &provider.ListRecycleResponse{Status: ok(), RecycleItems: slices.Clone(g.trash)}, nil
}

func (g *fakeGateway) RestoreRecycleItem(ctx context.Context, req *provider.RestoreRecycleItemRequest, _ ...grpc.CallOption) (*provider.RestoreRecycleItemResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.call(ctx, "RestoreRecycleItem")
	if err := g.deny(req.RestoreRef.GetPath()); err != nil {
		return nil, err
	}
	i := slices.IndexFunc(g.trash, func(it *provider.RecycleItem) bool { return it.Key == req.Key })
	if i < 0 {
		return &provider.RestoreRecycleItemResponse{Status: code(rpc.Code_CODE_NOT_FOUND)}, nil
	}
	// Back where it was deleted from, whatever the request says, as on EOS.
	f := g.trashed[req.Key]
	g.files[g.trash[i].Ref.Path] = f
	g.trash = slices.Delete(g.trash, i, i+1)
	return &provider.RestoreRecycleItemResponse{Status: ok()}, nil
}

func (g *fakeGateway) ListReceivedShares(ctx context.Context, req *collaboration.ListReceivedSharesRequest, _ ...grpc.CallOption) (*collaboration.ListReceivedSharesResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.call(ctx, "ListReceivedShares")
	return &collaboration.ListReceivedSharesResponse{Status: ok(), Shares: g.received}, nil
}

func (g *fakeGateway) ListShares(ctx context.Context, req *collaboration.ListSharesRequest, _ ...grpc.CallOption) (*collaboration.ListSharesResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.call(ctx, "ListShares")
	return &collaboration.ListSharesResponse{Status: ok(), Shares: g.shares}, nil
}

func (g *fakeGateway) ListPublicShares(ctx context.Context, req *link.ListPublicSharesRequest, _ ...grpc.CallOption) (*link.ListPublicSharesResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.call(ctx, "ListPublicShares")
	return &link.ListPublicSharesResponse{Status: ok(), Share: g.links}, nil
}

func (g *fakeGateway) GetShare(ctx context.Context, req *collaboration.GetShareRequest, _ ...grpc.CallOption) (*collaboration.GetShareResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.call(ctx, "GetShare")
	for _, s := range g.shares {
		if s.Id.OpaqueId == req.Ref.GetId().GetOpaqueId() {
			return &collaboration.GetShareResponse{Status: ok(), Share: s}, nil
		}
	}
	return &collaboration.GetShareResponse{Status: code(rpc.Code_CODE_NOT_FOUND)}, nil
}

func (g *fakeGateway) GetPublicShare(ctx context.Context, req *link.GetPublicShareRequest, _ ...grpc.CallOption) (*link.GetPublicShareResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.call(ctx, "GetPublicShare")
	for _, l := range g.links {
		if l.Id.OpaqueId == req.Ref.GetId().GetOpaqueId() {
			return &link.GetPublicShareResponse{Status: ok(), Share: l}, nil
		}
	}
	return &link.GetPublicShareResponse{Status: code(rpc.Code_CODE_NOT_FOUND)}, nil
}

func (g *fakeGateway) CreateShare(ctx context.Context, req *collaboration.CreateShareRequest, _ ...grpc.CallOption) (*collaboration.CreateShareResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.call(ctx, "CreateShare")
	s := &collaboration.Share{
		Id:          &collaboration.ShareId{OpaqueId: strconv.Itoa(g.nextID())},
		ResourceId:  req.ResourceInfo.Id,
		Permissions: req.Grant.Permissions,
		Grantee:     req.Grant.Grantee,
		Owner:       einstein.Id,
		Creator:     einstein.Id,
	}
	g.shares = append(g.shares, s)
	return &collaboration.CreateShareResponse{Status: ok(), Share: s}, nil
}

func (g *fakeGateway) CreatePublicShare(ctx context.Context, req *link.CreatePublicShareRequest, _ ...grpc.CallOption) (*link.CreatePublicShareResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.call(ctx, "CreatePublicShare")
	if req.ResourceInfo.ArbitraryMetadata == nil {
		panic("public share managers read the link name from the resource metadata")
	}
	n := g.nextID()
	l := &link.PublicShare{
		Id:                &link.PublicShareId{OpaqueId: strconv.Itoa(n)},
		Token:             "tok" + strconv.Itoa(n),
		ResourceId:        req.ResourceInfo.Id,
		Permissions:       req.Grant.Permissions,
		PasswordProtected: req.Grant.Password != "",
		Expiration:        req.Grant.Expiration,
		Owner:             einstein.Id,
	}
	g.links = append(g.links, l)
	return &link.CreatePublicShareResponse{Status: ok(), Share: l}, nil
}

func (g *fakeGateway) RemoveShare(ctx context.Context, req *collaboration.RemoveShareRequest, _ ...grpc.CallOption) (*collaboration.RemoveShareResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.call(ctx, "RemoveShare")
	n := len(g.shares)
	g.shares = slices.DeleteFunc(g.shares, func(s *collaboration.Share) bool { return s.Id.OpaqueId == req.Ref.GetId().GetOpaqueId() })
	if n == len(g.shares) {
		return &collaboration.RemoveShareResponse{Status: code(rpc.Code_CODE_NOT_FOUND)}, nil
	}
	return &collaboration.RemoveShareResponse{Status: ok()}, nil
}

func (g *fakeGateway) RemovePublicShare(ctx context.Context, req *link.RemovePublicShareRequest, _ ...grpc.CallOption) (*link.RemovePublicShareResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.call(ctx, "RemovePublicShare")
	n := len(g.links)
	g.links = slices.DeleteFunc(g.links, func(l *link.PublicShare) bool { return l.Id.OpaqueId == req.Ref.GetId().GetOpaqueId() })
	if n == len(g.links) {
		return &link.RemovePublicShareResponse{Status: code(rpc.Code_CODE_NOT_FOUND)}, nil
	}
	return &link.RemovePublicShareResponse{Status: ok()}, nil
}

func (g *fakeGateway) GetUser(ctx context.Context, req *userpb.GetUserRequest, _ ...grpc.CallOption) (*userpb.GetUserResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.call(ctx, "GetUser")
	for _, u := range g.users {
		if u.Id.OpaqueId == req.UserId.GetOpaqueId() {
			return &userpb.GetUserResponse{Status: ok(), User: u}, nil
		}
	}
	return &userpb.GetUserResponse{Status: code(rpc.Code_CODE_NOT_FOUND)}, nil
}

func (g *fakeGateway) GetUserByClaim(ctx context.Context, req *userpb.GetUserByClaimRequest, _ ...grpc.CallOption) (*userpb.GetUserByClaimResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.call(ctx, "GetUserByClaim")
	if u, found := g.users[req.Value]; found && req.Claim == "username" {
		return &userpb.GetUserByClaimResponse{Status: ok(), User: u}, nil
	}
	return &userpb.GetUserByClaimResponse{Status: code(rpc.Code_CODE_NOT_FOUND)}, nil
}

func (g *fakeGateway) GetGroupByClaim(ctx context.Context, req *grouppb.GetGroupByClaimRequest, _ ...grpc.CallOption) (*grouppb.GetGroupByClaimResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.call(ctx, "GetGroupByClaim")
	if req.Claim == "group_name" && req.Value == "physicists" {
		return &grouppb.GetGroupByClaimResponse{Status: ok(), Group: &grouppb.Group{Id: &grouppb.GroupId{OpaqueId: "physicists"}, GroupName: "physicists"}}, nil
	}
	return &grouppb.GetGroupByClaimResponse{Status: code(rpc.Code_CODE_NOT_FOUND)}, nil
}

func (g *fakeGateway) setLabel(ctx context.Context, method string, ref *provider.Reference, label string, set bool) (*rpc.Status, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.call(ctx, method)
	f, _, err := g.lookup(ref)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return code(rpc.Code_CODE_NOT_FOUND), nil
	}
	if f.info.ArbitraryMetadata == nil {
		f.info.ArbitraryMetadata = &provider.ArbitraryMetadata{Metadata: map[string]string{}}
	}
	key := "reva.labels." + einstein.Id.OpaqueId + "." + label
	if set {
		f.info.ArbitraryMetadata.Metadata[key] = "1"
	} else {
		delete(f.info.ArbitraryMetadata.Metadata, key)
	}
	return ok(), nil
}

func (g *fakeGateway) AddLabel(ctx context.Context, req *labels.AddLabelRequest, _ ...grpc.CallOption) (*labels.AddLabelResponse, error) {
	st, err := g.setLabel(ctx, "AddLabel", req.Ref, req.Label, true)
	return &labels.AddLabelResponse{Status: st}, err
}

func (g *fakeGateway) RemoveLabel(ctx context.Context, req *labels.RemoveLabelRequest, _ ...grpc.CallOption) (*labels.RemoveLabelResponse, error) {
	st, err := g.setLabel(ctx, "RemoveLabel", req.Ref, req.Label, false)
	return &labels.RemoveLabelResponse{Status: st}, err
}

// testResolver hands the gateway of the running test to service.Gateway.
type testResolver struct {
	service.Clients
	mu sync.Mutex
	gw gateway.GatewayAPIClient
}

func (r *testResolver) Gateway(context.Context) (gateway.GatewayAPIClient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.gw, nil
}

var (
	resolver     = &testResolver{}
	resolverOnce sync.Once
)

func useGateway(gw gateway.GatewayAPIClient) {
	resolverOnce.Do(func() { service.SetGlobal(resolver) })
	resolver.mu.Lock()
	resolver.gw = gw
	resolver.mu.Unlock()
}
