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

package ocdav

import (
	"context"
	"testing"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/spaces"
)

func TestResolveLinkTarget(t *testing.T) {
	const root = "/eos/user/e/einstein"
	tests := []struct {
		name    string
		link    string
		target  string
		windows bool
		want    string
		wantOK  bool
	}{
		{"sibling", root + "/a/l", "b", false, root + "/a/b", true},
		{"up to root", root + "/a/l", "..", false, root, true},
		{"up one, down", root + "/a/b/l", "../c/d.txt", false, root + "/a/c/d.txt", true},
		{"dot slash", root + "/l", "./x", false, root + "/x", true},
		{"escapes root", root + "/a/l", "../../x", false, "", false},
		{"escapes into a sibling space with same prefix", root + "/l", "../einstein2/x", false, "", false},
		{"absolute", root + "/l", root + "/x", false, "", false},
		{"self", root + "/a/l", "../a/l", false, "", false},
		{"empty", root + "/l", "", false, "", false},
		{"windows relative", root + "/a/l.lnk", `..\Docs\r.docx`, true, root + "/Docs/r.docx", true},
		{"windows escapes", root + "/l.lnk", `..\x`, true, "", false},
		{"windows drive", root + "/l.lnk", `C:\Users\x`, true, "", false},
		{"windows drive relative", root + "/l.lnk", `C:x`, true, "", false},
		{"windows unc", root + "/l.lnk", `\\server\share\x`, true, "", false},
		{"windows rooted", root + "/l.lnk", `\x`, true, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := resolveLinkTarget(tt.link, tt.target, root, tt.windows)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("got (%q, %v), want (%q, %v)", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestLinkRoot(t *testing.T) {
	root := "/eos/user/e/einstein"
	md := &provider.ResourceInfo{Path: root + "/a/l", Id: &provider.ResourceId{SpaceId: spaces.EncodeSpaceID(root)}}
	if got, ok := linkRoot(context.Background(), md); !ok || got != root {
		t.Errorf("space root: got (%q, %v)", got, ok)
	}

	pub := context.WithValue(context.Background(), ctxPublicLink, "tok")
	md = &provider.ResourceInfo{Path: "/public/tok/a/l", Id: &provider.ResourceId{SpaceId: spaces.EncodeSpaceID(root)}}
	if got, ok := linkRoot(pub, md); !ok || got != "/public/tok" {
		t.Errorf("public link root: got (%q, %v)", got, ok)
	}
	md = &provider.ResourceInfo{Path: "/public/tokx/a/l"}
	if _, ok := linkRoot(pub, md); ok {
		t.Errorf("public link root must match the whole token")
	}

	if _, ok := linkRoot(context.Background(), &provider.ResourceInfo{Path: "/x"}); ok {
		t.Errorf("no space id must not give a root")
	}
}

func TestMdToPropResponseLinks(t *testing.T) {
	root := "/eos/user/e/einstein"
	s := &svc{c: &Config{EnableLinkTargets: true}}
	ctx := appctx.ContextSetUser(context.Background(), &userv1beta1.User{
		Id:       &userv1beta1.UserId{OpaqueId: "einstein", Idp: "cern.ch"},
		Username: "einstein",
	})
	spaceID := spaces.EncodeSpaceID(root)
	parent := &provider.ResourceInfo{Path: root + "/a", Type: provider.ResourceType_RESOURCE_TYPE_CONTAINER}
	newLink := func(name, target string) *provider.ResourceInfo {
		return &provider.ResourceInfo{
			Path:   root + "/a/" + name,
			Id:     &provider.ResourceId{SpaceId: spaceID, OpaqueId: name},
			Type:   provider.ResourceType_RESOURCE_TYPE_SYMLINK,
			Target: target,
		}
	}
	mds := []*provider.ResourceInfo{
		newLink("in", "../b/my file.txt"),
		newLink("out", "../../x"),
		newLink("abs", "/eos/project/x"),
		newLink("abs-in-space", root+"/b"),
		{Path: root + "/a/f", Id: &provider.ResourceId{SpaceId: spaceID}, Type: provider.ResourceType_RESOURCE_TYPE_FILE},
	}
	pf := &propfindXML{Prop: propfindProps{
		{Space: _nsOwncloud, Local: _propLinkType},
		{Space: _nsOwncloud, Local: _propLinkTarget},
	}}
	links := s.resolveLinks(ctx, mds)

	tests := []struct {
		md         *provider.ResourceInfo
		wantType   string
		wantTarget string
	}{
		{mds[0], "symlink", "/remote.php/dav/files/einstein/b/my%20file.txt"},
		{mds[1], "symlink", ""},
		// no dav base in the context, as with the legacy /webdav endpoint
		{mds[2], "symlink", ""},
		{mds[3], "symlink", "/remote.php/dav/files/einstein/b"},
		{mds[4], "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.md.Path, func(t *testing.T) {
			res, err := s.mdToPropResponse(ctx, pf, tt.md, parent, "", "/remote.php/dav/files/einstein/a", nil, nil, links)
			if err != nil {
				t.Fatalf("mdToPropResponse failed: %v", err)
			}
			checkProp := func(name, want string) {
				status, value := findProp(t, res, name)
				if want == "" {
					if status != "HTTP/1.1 404 Not Found" {
						t.Errorf("%s: expected 404, got %q in %q", name, value, status)
					}
				} else if status != "HTTP/1.1 200 OK" || value != want {
					t.Errorf("%s: expected %q, got %q in %q", name, want, value, status)
				}
			}
			checkProp("oc:link-type", tt.wantType)
			checkProp("oc:link-target", tt.wantTarget)
		})
	}

	t.Run("disabled", func(t *testing.T) {
		s := &svc{c: &Config{}}
		if links := s.resolveLinks(ctx, mds); links != nil {
			t.Errorf("expected no links when disabled, got %v", links)
		}
	})
}

func TestSpaceHref(t *testing.T) {
	space := "/eos/project/c/cernbox"
	info := func(p string, typ provider.ResourceType) *provider.ResourceInfo {
		return &provider.ResourceInfo{
			Path: p,
			Type: typ,
			Id:   &provider.ResourceId{StorageId: "eosproject", SpaceId: spaces.EncodeSpaceID(space), OpaqueId: "1"},
		}
	}
	driveID := spaces.EncodeStorageSpaceID("eosproject", spaces.EncodeSpaceID(space))

	got, ok := spaceHref("/remote.php/dav", info(space+"/docs/my file.txt", provider.ResourceType_RESOURCE_TYPE_FILE))
	if want := "/remote.php/dav/spaces/" + driveID + "/docs/my%20file.txt"; !ok || got != want {
		t.Errorf("file: got (%q, %v), want %q", got, ok, want)
	}
	got, ok = spaceHref("/remote.php/dav", info(space+"/docs", provider.ResourceType_RESOURCE_TYPE_CONTAINER))
	if want := "/remote.php/dav/spaces/" + driveID + "/docs/"; !ok || got != want {
		t.Errorf("folder: got (%q, %v), want %q", got, ok, want)
	}
	if _, ok := spaceHref("/remote.php/dav", info("/eos/project/c/other/x", provider.ResourceType_RESOURCE_TYPE_FILE)); ok {
		t.Errorf("a path outside of its space must not get an href")
	}
	if _, ok := spaceHref("", info(space+"/x", provider.ResourceType_RESOURCE_TYPE_FILE)); ok {
		t.Errorf("no dav base must not give an href")
	}
	noStorage := info(space+"/x", provider.ResourceType_RESOURCE_TYPE_FILE)
	noStorage.Id.StorageId = ""
	if _, ok := spaceHref("/remote.php/dav", noStorage); ok {
		t.Errorf("no storage id must not give an href")
	}
}

func TestResolveLinksPublicAbsolute(t *testing.T) {
	s := &svc{c: &Config{EnableLinkTargets: true}}
	ctx := context.WithValue(context.Background(), ctxPublicLink, "tok")
	ctx = context.WithValue(ctx, ctxKeyDavBaseURI, "/remote.php/dav")
	md := &provider.ResourceInfo{
		Path:   "/public/tok/l",
		Type:   provider.ResourceType_RESOURCE_TYPE_SYMLINK,
		Target: "/public/tok/x",
	}
	links := s.resolveLinks(ctx, []*provider.ResourceInfo{md})
	if li := links[md.Path]; li == nil || li.TargetPath != "" || li.TargetHref != "" {
		t.Errorf("absolute targets must stay opaque in public links, got %+v", li)
	}
}
