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
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	mockgateway "github.com/cs3org/go-cs3apis/mocks/github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"github.com/cs3org/reva/v3/pkg/storage"
	"github.com/stretchr/testify/mock"
)

const trashSpaceRoot = "/winspaces/s/proj"

var (
	trashSpaceID   = spaces.EncodeStorageSpaceID("cephmount", spaces.EncodeSpaceID(trashSpaceRoot))
	trashSpaceHref = "/remote.php/dav/spaces/trash-bin/" + trashSpaceID
)

// serveSpacesTrashbin sends a request for the trash bin of the test space through the route of ocdav.go.
func serveSpacesTrashbin(t *testing.T, method, itemPath, body string, header http.Header) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, trashSpaceHref+itemPath, strings.NewReader(body))
	for k, v := range header {
		r.Header[k] = v
	}
	r = r.WithContext(appctx.ContextSetUser(r.Context(), &userv1beta1.User{
		Id:       &userv1beta1.UserId{OpaqueId: "alice", Idp: "cern.ch"},
		Username: "alice",
	}))
	w := httptest.NewRecorder()
	s := &svc{c: &Config{}, davHandler: &DavHandler{TrashbinHandler: &TrashbinHandler{}}}
	s.spacesTrashbin("/remote.php/dav/spaces")(w, r)
	return w
}

type trashEntry struct {
	href      string
	browsable bool
}

// readTrashEntries returns the decoded href of each response, in order, and whether it is marked as browsable.
func readTrashEntries(t *testing.T, w *httptest.ResponseRecorder) []trashEntry {
	t.Helper()
	var ms struct {
		Responses []struct {
			Href     string `xml:"href"`
			Propstat []struct {
				Status string `xml:"status"`
				Prop   struct {
					Browsable *string `xml:"trashbin-browsable"`
				} `xml:"prop"`
			} `xml:"propstat"`
		} `xml:"response"`
	}
	if err := xml.Unmarshal(w.Body.Bytes(), &ms); err != nil {
		t.Fatalf("cannot parse the multistatus: %v; body: %s", err, w.Body.String())
	}
	entries := make([]trashEntry, 0, len(ms.Responses))
	for _, r := range ms.Responses {
		href, err := url.PathUnescape(r.Href)
		if err != nil {
			t.Fatalf("cannot decode href %q: %v", r.Href, err)
		}
		e := trashEntry{href: href}
		for _, ps := range r.Propstat {
			if strings.Contains(ps.Status, "200") && ps.Prop.Browsable != nil && *ps.Prop.Browsable == "1" {
				e.browsable = true
			}
		}
		entries = append(entries, e)
	}
	return entries
}

func TestSpacesTrashbinListsAFolderOfTheBin(t *testing.T) {
	gw := mockgateway.NewMockGatewayAPIClient(t)
	var got *provider.ListRecycleRequest
	gw.On("ListRecycle", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		got = args.Get(1).(*provider.ListRecycleRequest)
	}).Return(&provider.ListRecycleResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
		RecycleItems: []*provider.RecycleItem{{
			Type:         provider.ResourceType_RESOURCE_TYPE_FILE,
			Key:          "20261123/docs/report.txt",
			Ref:          &provider.Reference{Path: trashSpaceRoot + "/docs/report.txt"},
			DeletionTime: &types.Timestamp{Seconds: 1},
		}},
	}, nil)
	stampGateway(gw)

	w := serveSpacesTrashbin(t, MethodPropfind, "/20261123/docs", "", http.Header{HeaderDepth: {"1"}})

	if w.Code != http.StatusMultiStatus {
		t.Fatalf("got status %d, want %d; body: %s", w.Code, http.StatusMultiStatus, w.Body.String())
	}
	if got.GetRef().GetPath() != trashSpaceRoot || got.GetKey() != "20261123/docs" {
		t.Errorf("got ListRecycle of %q with key %q, want %q with key %q", got.GetRef().GetPath(), got.GetKey(), trashSpaceRoot, "20261123/docs")
	}
	var hrefs []string
	for _, e := range readTrashEntries(t, w) {
		hrefs = append(hrefs, e.href)
	}
	// The listed folder comes first, so that a client knows where it is in the bin.
	want := []string{trashSpaceHref + "/20261123/docs/", trashSpaceHref + "/20261123/docs/report.txt"}
	if !slices.Equal(hrefs, want) {
		t.Errorf("got hrefs %q, want %q", hrefs, want)
	}
}

func TestSpacesTrashbinMarksBrowsableItems(t *testing.T) {
	gw := mockgateway.NewMockGatewayAPIClient(t)
	gw.On("ListRecycle", mock.Anything, mock.Anything).Return(&provider.ListRecycleResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
		RecycleItems: []*provider.RecycleItem{
			{
				Type:         provider.ResourceType_RESOURCE_TYPE_CONTAINER,
				Key:          "20261123/docs",
				Ref:          &provider.Reference{Path: trashSpaceRoot + "/docs"},
				DeletionTime: &types.Timestamp{Seconds: 1},
				Opaque: &types.Opaque{Map: map[string]*types.OpaqueEntry{
					storage.RecycleItemBrowsable: {Decoder: "plain", Value: []byte("true")},
				}},
			},
			// A folder of a flat recycle bin, as in EOS, cannot be opened.
			{
				Type:         provider.ResourceType_RESOURCE_TYPE_CONTAINER,
				Key:          "1a2b3c",
				Ref:          &provider.Reference{Path: trashSpaceRoot + "/old"},
				DeletionTime: &types.Timestamp{Seconds: 1},
			},
		},
	}, nil)
	stampGateway(gw)

	body := `<?xml version="1.0"?><d:propfind xmlns:d="DAV:" xmlns:oc="http://owncloud.org/ns">` +
		`<d:prop><d:resourcetype/><oc:trashbin-browsable/></d:prop></d:propfind>`
	w := serveSpacesTrashbin(t, MethodPropfind, "", body, http.Header{HeaderDepth: {"1"}})

	if w.Code != http.StatusMultiStatus {
		t.Fatalf("got status %d, want %d; body: %s", w.Code, http.StatusMultiStatus, w.Body.String())
	}
	want := []trashEntry{
		{href: trashSpaceHref + "/"},
		{href: trashSpaceHref + "/20261123/docs/", browsable: true},
		{href: trashSpaceHref + "/1a2b3c/"},
	}
	if got := readTrashEntries(t, w); !slices.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestSpacesTrashbinRestoresAndPurgesTheWholeKey(t *testing.T) {
	ok := &rpc.Status{Code: rpc.Code_CODE_OK}
	for name, key := range map[string]string{
		"key with a path": "20261123/docs/report.txt",
		"flat key":        "1a2b3c",
	} {
		t.Run(name+"/restore", func(t *testing.T) {
			gw := mockgateway.NewMockGatewayAPIClient(t)
			gw.On("Stat", mock.Anything, mock.Anything).Return(&provider.StatResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}, nil).Once()
			gw.On("Stat", mock.Anything, mock.Anything).Return(&provider.StatResponse{
				Status: ok,
				Info:   &provider.ResourceInfo{Id: &provider.ResourceId{StorageId: "cephmount", OpaqueId: "1"}},
			}, nil)
			var got string
			gw.On("RestoreRecycleItem", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
				got = args.Get(1).(*provider.RestoreRecycleItemRequest).GetKey()
			}).Return(&provider.RestoreRecycleItemResponse{Status: ok}, nil)
			stampGateway(gw)

			w := serveSpacesTrashbin(t, MethodMove, "/"+key, "", http.Header{
				HeaderDestination: {"https://cernbox.example.org/remote.php/dav/spaces/" + trashSpaceID + "/docs/report.txt"},
			})

			if w.Code != http.StatusCreated {
				t.Fatalf("got status %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
			}
			if got != key {
				t.Errorf("got key %q, want %q", got, key)
			}
		})

		t.Run(name+"/purge", func(t *testing.T) {
			gw := mockgateway.NewMockGatewayAPIClient(t)
			var got string
			gw.On("PurgeRecycle", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
				got = args.Get(1).(*provider.PurgeRecycleRequest).GetKey()
			}).Return(&provider.PurgeRecycleResponse{Status: ok}, nil)
			stampGateway(gw)

			w := serveSpacesTrashbin(t, http.MethodDelete, "/"+key, "", nil)

			if w.Code != http.StatusNoContent {
				t.Fatalf("got status %d, want %d; body: %s", w.Code, http.StatusNoContent, w.Body.String())
			}
			if got != key {
				t.Errorf("got key %q, want %q", got, key)
			}
		})
	}
}

func TestSpacesTrashbinRestoreReportsCollisions(t *testing.T) {
	gw := mockgateway.NewMockGatewayAPIClient(t)
	// The folder is already there: the storage merges into it, and reports what collides.
	gw.On("Stat", mock.Anything, mock.Anything).Return(&provider.StatResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
		Info:   &provider.ResourceInfo{Id: &provider.ResourceId{StorageId: "cephmount", OpaqueId: "1"}},
	}, nil)
	collisions, _ := json.Marshal(storage.RecycleCollisions{{Key: "20261123/docs/report.txt", Path: "docs/report.txt"}})
	var got *provider.RestoreRecycleItemRequest
	gw.On("RestoreRecycleItem", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		got = args.Get(1).(*provider.RestoreRecycleItemRequest)
	}).Return(&provider.RestoreRecycleItemResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_ALREADY_EXISTS},
		Opaque: &types.Opaque{Map: map[string]*types.OpaqueEntry{
			storage.RecycleRestoreCollisions: {Decoder: "json", Value: collisions},
		}},
	}, nil)
	stampGateway(gw)

	w := serveSpacesTrashbin(t, MethodMove, "/20261123/docs", "", http.Header{
		HeaderDestination: {"https://cernbox.example.org/remote.php/dav/spaces/" + trashSpaceID + "/docs"},
		HeaderOverwrite:   {"F"},
	})

	if w.Code != http.StatusPreconditionFailed {
		t.Fatalf("got status %d, want %d; body: %s", w.Code, http.StatusPreconditionFailed, w.Body.String())
	}
	if _, force := got.GetOpaque().GetMap()[storage.RecycleRestoreForce]; force {
		t.Error("got a forced restore, want a normal one")
	}
	type collision struct {
		Key  string `xml:"key"`
		Path string `xml:"path"`
	}
	var body struct {
		Collisions []collision `xml:"collisions>collision"`
	}
	if err := xml.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("cannot parse the error: %v; body: %s", err, w.Body.String())
	}
	want := []collision{{Key: "20261123/docs/report.txt", Path: "docs/report.txt"}}
	if !slices.Equal(body.Collisions, want) {
		t.Errorf("got collisions %+v, want %+v", body.Collisions, want)
	}
}

func TestSpacesTrashbinRestoreWithOverwriteForcesIt(t *testing.T) {
	gw := mockgateway.NewMockGatewayAPIClient(t)
	gw.On("Stat", mock.Anything, mock.Anything).Return(&provider.StatResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
		Info:   &provider.ResourceInfo{Id: &provider.ResourceId{StorageId: "cephmount", OpaqueId: "1"}},
	}, nil)
	var got *provider.RestoreRecycleItemRequest
	gw.On("RestoreRecycleItem", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		got = args.Get(1).(*provider.RestoreRecycleItemRequest)
	}).Return(&provider.RestoreRecycleItemResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}}, nil)
	stampGateway(gw)

	w := serveSpacesTrashbin(t, MethodMove, "/1a2b3c", "", http.Header{
		HeaderDestination: {"https://cernbox.example.org/remote.php/dav/spaces/" + trashSpaceID + "/docs/report.txt"},
		HeaderOverwrite:   {"T"},
	})

	if w.Code != http.StatusNoContent {
		t.Fatalf("got status %d, want %d; body: %s", w.Code, http.StatusNoContent, w.Body.String())
	}
	if _, force := got.GetOpaque().GetMap()[storage.RecycleRestoreForce]; !force {
		t.Error("got a normal restore, want a forced one")
	}
}
