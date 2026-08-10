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
	"strings"
	"testing"
	"time"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
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
		res, err := s.mdToPropResponse(ctx, pf, newMd(nil), parent, "", "/remote.php/dav/files/einstein", nil, nil)
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

		res, err := s.mdToPropResponse(ctx, pf, locked, parent, "", "/remote.php/dav/files/einstein", nil, nil)
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
