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

package ocdav

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	mockgateway "github.com/cs3org/go-cs3apis/mocks/github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/mock"
)

func TestActiveLock(t *testing.T) {
	inOneHour := &types.Timestamp{Seconds: uint64(time.Now().Add(time.Hour).Unix())}
	anHourAgo := &types.Timestamp{Seconds: uint64(time.Now().Add(-time.Hour).Unix())}

	tests := []struct {
		name     string
		lock     *provider.Lock
		contains []string
		want     string
	}{
		{
			name: "no lock",
			lock: nil,
			want: "",
		},
		{
			name: "invalid lock type",
			lock: &provider.Lock{LockId: "some-id", Type: provider.LockType_LOCK_TYPE_INVALID, Expiration: inOneHour},
			want: "",
		},
		{
			name: "expired lock",
			lock: &provider.Lock{LockId: "some-id", Type: provider.LockType_LOCK_TYPE_WRITE, Expiration: anHourAgo},
			want: "",
		},
		{
			name: "exclusive lock held by a user through an app",
			lock: &provider.Lock{
				LockId:       "some-id",
				Type:         provider.LockType_LOCK_TYPE_EXCL,
				User:         &userv1beta1.UserId{OpaqueId: "einstein", Idp: "cern.ch"},
				AppName:      "MicrosoftOffice",
				Expiration:   inOneHour,
				CreationTime: &types.Timestamp{Seconds: 1786432355},
			},
			contains: []string{
				"<d:lockscope><d:exclusive/></d:lockscope>",
				"<d:locktype><d:write/></d:locktype>",
				"<d:owner>einstein via MicrosoftOffice</d:owner>",
				"<d:locktoken><d:href>some-id</d:href></d:locktoken>",
				"<d:lockroot><d:href>/dav/files/einstein/doc.docx</d:href></d:lockroot>",
				"<oc:locktime>2026-08-11T07:12:35Z</oc:locktime>",
			},
		},
		{
			name: "shared lock held by an app only",
			lock: &provider.Lock{
				LockId:     "some-id",
				Type:       provider.LockType_LOCK_TYPE_SHARED,
				AppName:    "Collabora",
				Expiration: inOneHour,
			},
			contains: []string{
				"<d:lockscope><d:shared/></d:lockscope>",
				"<d:owner>Collabora</d:owner>",
			},
		},
		{
			name: "lock without expiration never times out",
			lock: &provider.Lock{
				LockId: "some-id",
				Type:   provider.LockType_LOCK_TYPE_WRITE,
				User:   &userv1beta1.UserId{OpaqueId: "einstein"},
			},
			contains: []string{"<d:timeout>Infinite</d:timeout>"},
		},
		{
			name: "owner is xml escaped",
			lock: &provider.Lock{
				LockId:  "some-id",
				Type:    provider.LockType_LOCK_TYPE_WRITE,
				AppName: "a<b>c",
			},
			contains: []string{"<d:owner>a&lt;b&gt;c</d:owner>"},
		},
	}

	s := &svc{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.activeLock(tt.lock, "/dav/files/einstein/doc.docx")
			if tt.contains == nil {
				if got != tt.want {
					t.Errorf("expected %q, got %q", tt.want, got)
				}
				return
			}
			if !strings.HasPrefix(got, "<d:activelock>") || !strings.HasSuffix(got, "</d:activelock>") {
				t.Errorf("expected an activelock element, got %q", got)
			}
			for _, c := range tt.contains {
				if !strings.Contains(got, c) {
					t.Errorf("expected %q in %q", c, got)
				}
			}
		})
	}
}

func TestMdToPropResponseLockDiscovery(t *testing.T) {
	s := &svc{c: &Config{}}
	ctx := appctx.ContextSetUser(context.Background(), &userv1beta1.User{
		Id:       &userv1beta1.UserId{OpaqueId: "einstein", Idp: "cern.ch"},
		Username: "einstein",
	})
	parent := &provider.ResourceInfo{
		Path: "/home",
		Type: provider.ResourceType_RESOURCE_TYPE_CONTAINER,
	}
	// proto messages must not be copied, so build a fresh one per subtest
	newMd := func(lock *provider.Lock) *provider.ResourceInfo {
		return &provider.ResourceInfo{
			Path:          "/home/doc.odt",
			Type:          provider.ResourceType_RESOURCE_TYPE_FILE,
			PermissionSet: &provider.ResourcePermissions{Stat: true, InitiateFileDownload: true},
			Lock:          lock,
		}
	}
	pf := &propfindXML{Prop: propfindProps{{Space: _nsDav, Local: "lockdiscovery"}}}

	t.Run("unlocked resource reports an empty lockdiscovery", func(t *testing.T) {
		res, err := s.mdToPropResponse(ctx, pf, newMd(nil), parent, "", "/remote.php/dav/files/einstein", nil, nil, nil)
		if err != nil {
			t.Fatalf("mdToPropResponse failed: %v", err)
		}
		status, value := findProp(t, res, "d:lockdiscovery")
		if status != "HTTP/1.1 200 OK" || value != "" {
			t.Errorf("expected an empty lockdiscovery in the 200 propstat, got %q in %q", value, status)
		}
	})

	t.Run("locked resource reports the active lock", func(t *testing.T) {
		locked := newMd(&provider.Lock{
			LockId:       "opaquelocktoken:abcd",
			Type:         provider.LockType_LOCK_TYPE_WRITE,
			User:         &userv1beta1.UserId{OpaqueId: "einstein", Idp: "cern.ch"},
			AppName:      "MicrosoftOffice",
			Expiration:   &types.Timestamp{Seconds: uint64(time.Now().Add(time.Hour).Unix())},
			CreationTime: &types.Timestamp{Seconds: 1786432355},
		})

		res, err := s.mdToPropResponse(ctx, pf, locked, parent, "", "/remote.php/dav/files/einstein", nil, nil, nil)
		if err != nil {
			t.Fatalf("mdToPropResponse failed: %v", err)
		}
		status, value := findProp(t, res, "d:lockdiscovery")
		if status != "HTTP/1.1 200 OK" {
			t.Errorf("expected the lock in the 200 propstat, got %q", status)
		}
		for _, want := range []string{
			"<d:owner>einstein via MicrosoftOffice</d:owner>",
			"<d:locktoken><d:href>opaquelocktoken:abcd</d:href></d:locktoken>",
			"<d:lockroot><d:href>/remote.php/dav/files/einstein/doc.odt</d:href></d:lockroot>",
			"<oc:locktime>2026-08-11T07:12:35Z</oc:locktime>",
		} {
			if !strings.Contains(value, want) {
				t.Errorf("expected %q in %q", want, value)
			}
		}
		if res.Href != "/remote.php/dav/files/einstein/doc.odt" {
			t.Errorf("expected the lockroot to match the response href, got %q", res.Href)
		}
	})
}

// findProp returns the propstat status and the value of the named property.
func findProp(t *testing.T, res *responseXML, name string) (string, string) {
	t.Helper()

	for _, propstat := range res.Propstat {
		for _, prop := range propstat.Prop {
			if prop.XMLName.Local == name {
				return propstat.Status, string(prop.InnerXML)
			}
		}
	}
	t.Fatalf("%s not found in the response", name)
	return "", ""
}

func TestActiveLockTimeoutCountsDown(t *testing.T) {
	s := &svc{}
	got := s.activeLock(&provider.Lock{
		LockId:     "some-id",
		Type:       provider.LockType_LOCK_TYPE_WRITE,
		Expiration: &types.Timestamp{Seconds: uint64(time.Now().Add(30 * time.Minute).Unix())},
	}, "/dav/files/einstein/doc.docx")

	// allow for the second that may elapse while running the test
	if !strings.Contains(got, "<d:timeout>Second-1800</d:timeout>") && !strings.Contains(got, "<d:timeout>Second-1799</d:timeout>") {
		t.Errorf("expected a timeout of roughly 1800 seconds, got %q", got)
	}
}

func TestGetResourceInfosDepthOneOnFile(t *testing.T) {
	inFolder := &provider.ResourceInfo{
		Id:   &provider.ResourceId{StorageId: "eos", OpaqueId: "2"},
		Type: provider.ResourceType_RESOURCE_TYPE_FILE,
		Path: "/eos/user/a/alice/report.odt",
	}
	folder := &provider.ResourceInfo{
		Id:   &provider.ResourceId{StorageId: "eos", OpaqueId: "1"},
		Type: provider.ResourceType_RESOURCE_TYPE_CONTAINER,
		Path: "/eos/user/a/alice",
	}
	shareRoot := &provider.ResourceInfo{
		Id:   &provider.ResourceId{StorageId: "eos", OpaqueId: "3"},
		Type: provider.ResourceType_RESOURCE_TYPE_FILE,
		Path: "/report.odt",
	}

	tests := []struct {
		name       string
		file       *provider.ResourceInfo
		parent     *provider.ResourceInfo
		parentCode rpc.Code
		wantParent *provider.ResourceInfo
		wantInfos  int
		wantHref   string
	}{
		{name: "file in a folder", file: inFolder, parent: folder, parentCode: rpc.Code_CODE_OK, wantParent: folder, wantInfos: 2, wantHref: "/dav/spaces/sp"},
		{name: "single-file share root", file: shareRoot, parent: shareRoot, parentCode: rpc.Code_CODE_OK, wantParent: shareRoot, wantInfos: 2, wantHref: "/dav/spaces/sp/report.odt"},
		{name: "file in a folder the user cannot read", file: inFolder, parentCode: rpc.Code_CODE_PERMISSION_DENIED, wantParent: inFolder, wantInfos: 2, wantHref: "/dav/spaces/sp/report.odt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw := mockgateway.NewMockGatewayAPIClient(t)
			gw.On("Stat", mock.Anything, mock.Anything).Return(&provider.StatResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, Info: tt.file}, nil).Once()
			gw.On("Stat", mock.Anything, mock.Anything).Return(&provider.StatResponse{Status: &rpc.Status{Code: tt.parentCode}, Info: tt.parent}, nil).Once()
			stampGateway(gw)

			r := httptest.NewRequest("PROPFIND", "/dav/spaces/sp/report.odt", nil)
			r.Header.Set(HeaderDepth, "1")
			s := &svc{c: &Config{}}
			parent, infos, href, ok := s.getResourceInfos(context.Background(), httptest.NewRecorder(), r, propfindXML{}, &provider.Reference{ResourceId: tt.file.Id}, zerolog.Nop())
			if !ok {
				t.Fatal("getResourceInfos failed")
			}
			if parent != tt.wantParent || len(infos) != tt.wantInfos || href != tt.wantHref {
				t.Errorf("got parent %s, %d infos, href %q; want parent %s, %d infos, href %q", parent.Path, len(infos), href, tt.wantParent.Path, tt.wantInfos, tt.wantHref)
			}
		})
	}
}

// Web expects a depth 1 PROPFIND on a file to return the file two times. This must
// also be true when the user cannot access the parent folder, as on a single-file share.
func TestPropfindDepthOneOnFileReturnsFileTwice(t *testing.T) {
	perms := &provider.ResourcePermissions{Stat: true, InitiateFileDownload: true}
	file := &provider.ResourceInfo{
		Id:            &provider.ResourceId{StorageId: "eos", OpaqueId: "2"},
		Type:          provider.ResourceType_RESOURCE_TYPE_FILE,
		Path:          "/eos/user/a/alice/report.odt",
		PermissionSet: perms,
	}
	folder := &provider.ResourceInfo{
		Id:            &provider.ResourceId{StorageId: "eos", OpaqueId: "1"},
		Type:          provider.ResourceType_RESOURCE_TYPE_CONTAINER,
		Path:          "/eos/user/a/alice",
		PermissionSet: perms,
	}

	tests := []struct {
		name      string
		parentRes *provider.StatResponse
	}{
		{name: "file in a folder", parentRes: &provider.StatResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, Info: folder}},
		// the parent path of a single-file share root resolves to the shared file itself
		{name: "single-file share root", parentRes: &provider.StatResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, Info: file}},
		{name: "parent stat denied", parentRes: &provider.StatResponse{Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED}}},
		{name: "parent not found", parentRes: &provider.StatResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw := mockgateway.NewMockGatewayAPIClient(t)
			gw.On("Stat", mock.Anything, mock.Anything).Return(&provider.StatResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, Info: file}, nil).Once()
			gw.On("Stat", mock.Anything, mock.Anything).Return(tt.parentRes, nil).Once()
			gw.On("ListPublicShares", mock.Anything, mock.Anything).Return(&link.ListPublicSharesResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}}, nil)
			gw.On("ListShares", mock.Anything, mock.Anything).Return(&collaboration.ListSharesResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}}, nil)
			stampGateway(gw)

			body := `<?xml version="1.0"?><d:propfind xmlns:d="DAV:"><d:prop><d:lockdiscovery/></d:prop></d:propfind>`
			r := httptest.NewRequest(MethodPropfind, "/eos/user/a/alice/report.odt", strings.NewReader(body))
			r.Header.Set(HeaderDepth, "1")
			r = r.WithContext(appctx.ContextSetUser(r.Context(), &userv1beta1.User{
				Id:       &userv1beta1.UserId{OpaqueId: "alice", Idp: "cern.ch"},
				Username: "alice",
			}))
			w := httptest.NewRecorder()
			s := &svc{c: &Config{}}
			s.handlePathPropfind(w, r, "")

			if w.Code != http.StatusMultiStatus {
				t.Fatalf("got status %d, want %d; body: %s", w.Code, http.StatusMultiStatus, w.Body.String())
			}
			if n := strings.Count(w.Body.String(), "<d:href>/eos/user/a/alice/report.odt</d:href>"); n != 2 {
				t.Errorf("got the file %d times, want 2; body: %s", n, w.Body.String())
			}
		})
	}
}

// Only the parent of the file may be inaccessible: a denied stat on the file itself must still give a 403.
func TestPropfindDepthOneOnInaccessibleFileIsForbidden(t *testing.T) {
	gw := mockgateway.NewMockGatewayAPIClient(t)
	gw.On("Stat", mock.Anything, mock.Anything).Return(&provider.StatResponse{Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED}}, nil).Once()
	stampGateway(gw)

	r := httptest.NewRequest(MethodPropfind, "/eos/user/b/bob/report.odt", nil)
	r.Header.Set(HeaderDepth, "1")
	r = r.WithContext(appctx.ContextSetUser(r.Context(), &userv1beta1.User{
		Id:       &userv1beta1.UserId{OpaqueId: "alice", Idp: "cern.ch"},
		Username: "alice",
	}))
	w := httptest.NewRecorder()
	s := &svc{c: &Config{}}
	s.handlePathPropfind(w, r, "")

	if w.Code != http.StatusForbidden {
		t.Errorf("got status %d, want %d; body: %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}
