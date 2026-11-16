package ocdav

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	mockgateway "github.com/cs3org/go-cs3apis/mocks/github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/mock"
)

func TestNeedsShareTypes(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{
			name: "explicit share-types request",
			body: `<?xml version="1.0"?><d:propfind xmlns:d="DAV:" xmlns:oc="http://owncloud.org/ns"><d:prop><oc:share-types/></d:prop></d:propfind>`,
			want: true,
		},
		{
			name: "explicit request with other props",
			body: `<?xml version="1.0"?><d:propfind xmlns:d="DAV:" xmlns:oc="http://owncloud.org/ns"><d:prop><oc:fileid/><oc:share-types/><d:getetag/></d:prop></d:propfind>`,
			want: true,
		},
		{
			name: "allprop does not request share-types",
			body: `<?xml version="1.0"?><d:propfind xmlns:d="DAV:"><d:allprop/></d:propfind>`,
			want: false,
		},
		{
			name: "empty prop body (treated as allprop)",
			body: ``,
			want: false,
		},
		{
			name: "other explicit props only",
			body: `<?xml version="1.0"?><d:propfind xmlns:d="DAV:" xmlns:oc="http://owncloud.org/ns"><d:prop><oc:fileid/><d:getetag/></d:prop></d:propfind>`,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var pf propfindXML
			if tt.body != "" {
				if err := xml.NewDecoder(strings.NewReader(tt.body)).Decode(&pf); err != nil {
					t.Fatalf("failed to decode body: %v", err)
				}
			}
			if got := needsShareTypes(pf); got != tt.want {
				t.Errorf("needsShareTypes() = %v, want %v", got, tt.want)
			}
		})
	}
}

// A PROPFIND that does not ask for oc:share-types must not trigger the share
// lookups; one that does must trigger both. propfindResponse is called directly
// so that only the share-lookup behaviour is under test.
func TestPropfindShareLookupsOnlyForShareTypes(t *testing.T) {
	perms := &provider.ResourcePermissions{Stat: true, InitiateFileDownload: true}
	file := &provider.ResourceInfo{
		Id:            &provider.ResourceId{StorageId: "eos", OpaqueId: "2"},
		Type:          provider.ResourceType_RESOURCE_TYPE_FILE,
		Path:          "/eos/user/a/alice/report.odt",
		PermissionSet: perms,
	}
	parent := &provider.ResourceInfo{
		Id:            &provider.ResourceId{StorageId: "eos", OpaqueId: "1"},
		Type:          provider.ResourceType_RESOURCE_TYPE_CONTAINER,
		Path:          "/eos/user/a/alice",
		PermissionSet: perms,
	}
	user := &userv1beta1.User{Id: &userv1beta1.UserId{OpaqueId: "alice", Idp: "cern.ch"}, Username: "alice"}

	tests := []struct {
		name        string
		body        string
		wantLookups bool
	}{
		{
			name:        "explicit share-types triggers the lookups",
			body:        `<?xml version="1.0"?><d:propfind xmlns:d="DAV:" xmlns:oc="http://owncloud.org/ns"><d:prop><oc:share-types/></d:prop></d:propfind>`,
			wantLookups: true,
		},
		{
			name:        "allprop skips the lookups",
			body:        `<?xml version="1.0"?><d:propfind xmlns:d="DAV:"><d:allprop/></d:propfind>`,
			wantLookups: false,
		},
		{
			name:        "other explicit props skip the lookups",
			body:        `<?xml version="1.0"?><d:propfind xmlns:d="DAV:"><d:prop><d:getetag/></d:prop></d:propfind>`,
			wantLookups: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw := mockgateway.NewMockGatewayAPIClient(t)
			if tt.wantLookups {
				gw.On("ListPublicShares", mock.Anything, mock.Anything).Return(&link.ListPublicSharesResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}}, nil).Once()
				gw.On("ListShares", mock.Anything, mock.Anything).Return(&collaboration.ListSharesResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}}, nil).Once()
			}
			stampGateway(gw)

			var pf propfindXML
			if err := xml.NewDecoder(strings.NewReader(tt.body)).Decode(&pf); err != nil {
				t.Fatalf("failed to decode body: %v", err)
			}

			r := httptest.NewRequest(MethodPropfind, "/eos/user/a/alice/report.odt", nil)
			r = r.WithContext(appctx.ContextSetUser(r.Context(), user))
			w := httptest.NewRecorder()
			s := &svc{c: &Config{}}
			s.propfindResponse(r.Context(), w, r, "", "/dav/spaces/sp/report.odt", pf, parent, []*provider.ResourceInfo{file}, zerolog.Nop())

			if w.Code != http.StatusMultiStatus {
				t.Fatalf("got status %d, want %d; body: %s", w.Code, http.StatusMultiStatus, w.Body.String())
			}
			// The mock fails the test on any unexpected call, so a missing
			// expectation on the non-lookup cases asserts the lookups were skipped.
			gw.AssertExpectations(t)
		})
	}
}
