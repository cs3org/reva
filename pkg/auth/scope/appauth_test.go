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

package scope

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	appregistry "github.com/cs3org/go-cs3apis/cs3/app/registry/v1beta1"
	appauthpb "github.com/cs3org/go-cs3apis/cs3/auth/applications/v1beta1"
	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	labels "github.com/cs3org/go-cs3apis/cs3/labels/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	prefpb "github.com/cs3org/go-cs3apis/cs3/preferences/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registry "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

const testRoot = "/eos/user/e/einstein/thesis"

var (
	inRoot  = &provider.Reference{Path: testRoot + "/chapter1.tex"}
	outRoot = &provider.Reference{Path: "/eos/user/e/einstein/private.tex"}
	byID    = &provider.ResourceId{StorageId: "eoshome-e", OpaqueId: "42"}
	shareID = &collaboration.ShareReference{Spec: &collaboration.ShareReference_Id{Id: &collaboration.ShareId{OpaqueId: "7"}}}
	linkID  = &link.PublicShareReference{Spec: &link.PublicShareReference_Id{Id: &link.PublicShareId{OpaqueId: "8"}}}
)

// catalogueExamples has one populated request per request type in the
// catalogue. inRoots is the verdict of the verifier alone for an app password
// rooted at testRoot: true for a path under it or a request without a
// reference, false for a reference by id (left to the interceptor) or for
// something the roots cannot be checked against.
var catalogueExamples = []struct {
	perm    string
	req     any
	inRoots bool
}{
	{PermFilesList, &provider.StatRequest{Ref: inRoot}, true},
	{PermFilesList, &provider.ListContainerRequest{Ref: inRoot}, true},
	{PermFilesList, &provider.ListContainerStreamRequest{Ref: inRoot}, true},
	{PermFilesList, &provider.GetPathRequest{ResourceId: byID}, false},
	{PermFilesList, &provider.GetQuotaRequest{Ref: inRoot}, true},
	{PermFilesList, &gateway.GetQuotaRequest{Ref: inRoot}, true},
	{PermFilesList, &provider.ListStorageSpacesRequest{}, true},
	{PermFilesList, &provider.GetHomeRequest{}, true},
	{PermFilesList, &provider.GetLockRequest{Ref: inRoot}, true},
	{PermFilesList, &registry.GetStorageProvidersRequest{Ref: inRoot}, true},
	{PermFilesList, &registry.ListStorageProvidersRequest{}, true},
	{PermFilesList, &registry.GetHomeRequest{}, true},
	{PermFilesRead, &provider.InitiateFileDownloadRequest{Ref: inRoot}, true},
	{PermFilesWrite, &provider.InitiateFileUploadRequest{Ref: inRoot}, true},
	{PermFilesWrite, &provider.TouchFileRequest{Ref: inRoot}, true},
	{PermFilesWrite, &provider.CreateContainerRequest{Ref: inRoot}, true},
	{PermFilesWrite, &provider.SetArbitraryMetadataRequest{Ref: inRoot}, true},
	{PermFilesWrite, &provider.UnsetArbitraryMetadataRequest{Ref: inRoot}, true},
	{PermFilesWrite, &provider.SetLockRequest{Ref: inRoot}, true},
	{PermFilesWrite, &provider.RefreshLockRequest{Ref: inRoot}, true},
	{PermFilesWrite, &provider.UnlockRequest{Ref: inRoot}, true},
	{PermFilesWrite, &provider.CreateReferenceRequest{Ref: inRoot, TargetUri: "cs3:eoshome-e/43"}, false},
	{PermFilesWrite, &provider.CreateSymlinkRequest{Ref: inRoot, Target: "/eos/user/e/einstein"}, false},
	{PermFilesMove, &provider.MoveRequest{Source: inRoot, Destination: &provider.Reference{Path: testRoot + "/chapter2.tex"}}, true},
	{PermFilesDelete, &provider.DeleteRequest{Ref: inRoot}, true},
	{PermVersionsList, &provider.ListFileVersionsRequest{Ref: inRoot}, true},
	{PermVersionsRestore, &provider.RestoreFileVersionRequest{Ref: inRoot, Key: "1"}, true},
	{PermTrashList, &provider.ListRecycleRequest{Ref: &provider.Reference{Path: "/eos/user/e/einstein"}}, true},
	{PermTrashList, &provider.ListRecycleStreamRequest{Ref: &provider.Reference{Path: "/eos/user/e/einstein"}}, true},
	{PermTrashRestore, &provider.RestoreRecycleItemRequest{Ref: &provider.Reference{Path: "/eos/user/e/einstein"}, Key: "1", RestoreRef: inRoot}, true},
	{PermTrashPurge, &provider.PurgeRecycleRequest{Ref: &provider.Reference{Path: "/eos/user/e/einstein"}, Key: "1"}, false},
	{PermSharesList, &collaboration.ListSharesRequest{}, true},
	{PermSharesList, &collaboration.GetShareRequest{Ref: shareID}, true},
	{PermSharesList, &collaboration.ListReceivedSharesRequest{}, true},
	{PermSharesList, &collaboration.GetReceivedShareRequest{Ref: shareID}, true},
	{PermSharesList, &link.ListPublicSharesRequest{}, true},
	{PermSharesList, &link.GetPublicShareRequest{Ref: linkID}, true},
	{PermSharesList, &link.GetPublicShareByTokenRequest{Token: "abc"}, true},
	{PermSharesList, &ocm.ListOCMSharesRequest{}, true},
	{PermSharesList, &ocm.GetOCMShareRequest{}, true},
	{PermSharesList, &ocm.ListReceivedOCMSharesRequest{}, true},
	{PermSharesList, &ocm.GetReceivedOCMShareRequest{}, true},
	{PermSharesList, &provider.ListGrantsRequest{Ref: inRoot}, true},
	{PermSharesManage, &collaboration.CreateShareRequest{ResourceInfo: &provider.ResourceInfo{Id: byID, Path: inRoot.Path}}, false},
	{PermSharesManage, &collaboration.UpdateShareRequest{Ref: shareID}, false},
	{PermSharesManage, &collaboration.RemoveShareRequest{Ref: shareID}, false},
	{PermSharesManage, &collaboration.UpdateReceivedShareRequest{Share: &collaboration.ReceivedShare{Share: &collaboration.Share{Id: &collaboration.ShareId{OpaqueId: "7"}}}}, false},
	{PermSharesManage, &link.CreatePublicShareRequest{ResourceInfo: &provider.ResourceInfo{Id: byID, Path: inRoot.Path}}, false},
	{PermSharesManage, &link.UpdatePublicShareRequest{Ref: linkID}, false},
	{PermSharesManage, &link.RemovePublicShareRequest{Ref: linkID}, false},
	{PermSharesManage, &provider.AddGrantRequest{Ref: inRoot}, true},
	{PermSharesManage, &provider.UpdateGrantRequest{Ref: inRoot}, true},
	{PermSharesManage, &provider.RemoveGrantRequest{Ref: inRoot}, true},
	{PermSharesManage, &provider.DenyGrantRequest{Ref: inRoot}, true},
	{PermDirectorySearch, &userpb.FindUsersRequest{}, true},
	{PermDirectorySearch, &grouppb.FindGroupsRequest{}, true},
	{PermDirectorySearch, &grouppb.GetMembersRequest{}, true},
	{PermDirectorySearch, &invitepb.FindAcceptedUsersRequest{}, true},
	{PermDirectorySearch, &invitepb.GetAcceptedUserRequest{}, true},
	{PermLabelsRead, &labels.ListLabelsRequest{}, true},
	{PermLabelsRead, &labels.ListResourcesForLabelRequest{}, true},
	{PermLabelsWrite, &labels.AddLabelRequest{Ref: inRoot}, true},
	{PermLabelsWrite, &labels.RemoveLabelRequest{Ref: inRoot}, true},
	{PermAppsOpen, &gateway.OpenInAppRequest{Ref: inRoot}, true},
	{PermAppsOpen, &appprovider.OpenInAppRequest{ResourceInfo: &provider.ResourceInfo{Id: byID, Path: inRoot.Path}}, false},
	{PermAppsOpen, &appregistry.GetAppProvidersRequest{ResourceInfo: &provider.ResourceInfo{Id: byID}}, true},
	{PermAppsOpen, &appregistry.ListAppProvidersRequest{}, true},
	{PermAppsOpen, &appregistry.ListSupportedMimeTypesRequest{}, true},
	{PermAppsOpen, &appregistry.GetDefaultAppProviderForMimeTypeRequest{}, true},
	{PermPreferencesWrite, &prefpb.SetKeyRequest{}, true},
}

func appAuthScopes(t *testing.T, s AppAuthScope) map[string]*authpb.Scope {
	t.Helper()
	scopes, err := AddAppAuthScope(&s, nil)
	if err != nil {
		t.Fatalf("AddAppAuthScope: %v", err)
	}
	return scopes
}

func verify(t *testing.T, scopes map[string]*authpb.Scope, req any) bool {
	t.Helper()
	ok, err := VerifyScope(context.Background(), scopes, req)
	if err != nil {
		t.Fatalf("VerifyScope: %v", err)
	}
	return ok
}

func TestAppAuthCatalogue(t *testing.T) {
	covered := map[reflect.Type]bool{}
	for _, e := range catalogueExamples {
		typ := reflect.TypeOf(e.req)
		covered[typ] = true
		t.Run(typ.Elem().Name(), func(t *testing.T) {
			if got := requestPermission[typ]; got != e.perm {
				t.Fatalf("catalogue maps it to %q, want %q", got, e.perm)
			}
			others := slices.DeleteFunc(AppAuthPermissions(), func(p string) bool { return p == e.perm })
			cases := []struct {
				name  string
				scope AppAuthScope
				want  bool
			}{
				{"granted", AppAuthScope{Client: ClientWebDAV, Permissions: []string{e.perm}}, true},
				{"every other permission", AppAuthScope{Client: ClientWebDAV, Permissions: others}, false},
				{"wildcard", AppAuthScope{Client: ClientSync, Permissions: []string{PermAll}}, true},
				{"granted with roots", AppAuthScope{Client: ClientWebDAV, Permissions: []string{e.perm}, Roots: []string{testRoot}}, e.inRoots},
			}
			for _, c := range cases {
				if got := verify(t, appAuthScopes(t, c.scope), e.req); got != c.want {
					t.Errorf("%s: VerifyScope = %v, want %v", c.name, got, c.want)
				}
			}
		})
	}
	for typ := range requestPermission {
		if !covered[typ] {
			t.Errorf("no example for %s", typ)
		}
	}
}

func TestAppAuthAlwaysAndNever(t *testing.T) {
	none := appAuthScopes(t, AppAuthScope{Client: ClientWebDAV, Permissions: []string{}, Roots: []string{testRoot}})
	for typ := range alwaysAllowed {
		if !verify(t, none, reflect.New(typ.Elem()).Interface()) {
			t.Errorf("%s must be allowed to any app password", typ)
		}
	}

	all := appAuthScopes(t, AppAuthScope{Client: ClientSync, Permissions: []string{PermAll}})
	never := []any{&adminpb.ListServicesRequest{}, MethodResource("/reva.admin.v1beta1.AdminAPI/InvokeStream")}
	for typ := range neverAllowed {
		never = append(never, reflect.New(typ.Elem()).Interface())
	}
	for _, req := range never {
		if verify(t, all, req) {
			t.Errorf("%s must be refused even to a wildcard app password", requestName(req))
		}
	}
	for _, req := range []any{&appauthpb.GenerateAppPasswordRequest{}, &appauthpb.ListAppPasswordsRequest{}, &appauthpb.InvalidateAppPasswordRequest{}, &adminpb.RequestAdminRequest{}} {
		if !neverAllowed[reflect.TypeOf(req)] {
			t.Errorf("%T must be in the never list", req)
		}
	}
}

// A request type the catalogue does not know is denied to a restricted app
// password and left to the wildcard.
func TestAppAuthUnknownRequest(t *testing.T) {
	unknown := &types.Opaque{}
	if verify(t, appAuthScopes(t, AppAuthScope{Client: ClientWebDAV, Permissions: AppAuthPermissions()}), unknown) {
		t.Error("an unknown request must be refused to a restricted app password")
	}
	if !verify(t, appAuthScopes(t, AppAuthScope{Client: ClientSync, Permissions: []string{PermAll}}), unknown) {
		t.Error("an unknown request must be allowed to a wildcard app password")
	}
}

// Every request reachable through the gateway, or sent by the gateway to a
// provider, must be classified, so that a new CS3 method is not forgotten.
func TestAppAuthCoversEveryRequest(t *testing.T) {
	services := []protoreflect.FullName{
		"cs3.gateway.v1beta1.GatewayAPI",
		"cs3.storage.provider.v1beta1.ProviderAPI",
		"cs3.storage.provider.v1beta1.SpacesAPI",
		"cs3.storage.registry.v1beta1.RegistryAPI",
		"cs3.sharing.collaboration.v1beta1.CollaborationAPI",
		"cs3.sharing.link.v1beta1.LinkAPI",
		"cs3.sharing.ocm.v1beta1.OcmAPI",
		"cs3.identity.user.v1beta1.UserAPI",
		"cs3.identity.group.v1beta1.GroupAPI",
		"cs3.identity.tenant.v1beta1.TenantAPI",
		"cs3.preferences.v1beta1.PreferencesAPI",
		"cs3.permissions.v1beta1.PermissionsAPI",
		"cs3.labels.v1beta1.LabelsAPI",
		"cs3.app.registry.v1beta1.RegistryAPI",
		"cs3.app.provider.v1beta1.ProviderAPI",
		"cs3.auth.applications.v1beta1.ApplicationsAPI",
		"cs3.auth.provider.v1beta1.ProviderAPI",
		"cs3.auth.registry.v1beta1.RegistryAPI",
		"cs3.ocm.invite.v1beta1.InviteAPI",
		"cs3.ocm.provider.v1beta1.ProviderAPI",
		"cs3.ocm.incoming.v1beta1.OcmIncomingAPI",
		"cs3.ocm.core.v1beta1.OcmCoreAPI",
		"cs3.tx.v1beta1.TxAPI",
		"reva.admin.v1beta1.AdminAPI",
		"reva.control.v1beta1.Control",
	}
	for _, name := range services {
		d, err := protoregistry.GlobalFiles.FindDescriptorByName(name)
		if err != nil {
			t.Fatalf("service %s is not registered: %v", name, err)
		}
		methods := d.(protoreflect.ServiceDescriptor).Methods()
		for i := range methods.Len() {
			m := methods.Get(i)
			method := MethodResource(fmt.Sprintf("/%s/%s", name, m.Name()))
			mt, err := protoregistry.GlobalTypes.FindMessageByName(m.Input().FullName())
			if err != nil {
				t.Fatalf("%s: %v", method, err)
			}
			req := mt.Zero().Interface()
			typ := reflect.TypeOf(req)
			if requestType(method) != typ {
				t.Errorf("%s: request type %v, want %v", method, requestType(method), typ)
			}
			_, inCatalogue := requestPermission[typ]
			if !inCatalogue && !alwaysAllowed[typ] && !neverAllowed[typ] && !isAdminResource(req) {
				t.Errorf("%s: %v is in none of the catalogue, the always list and the never list", method, typ)
			}
		}
	}
}

func TestAppAuthPathRule(t *testing.T) {
	paths := []string{"/remote.php/webdav/a", "/remote.php/dav/files/einstein", "/ocs/v2.php/cloud/user", "/data/simple/x"}
	for _, kind := range ClientKinds() {
		scopes := appAuthScopes(t, AppAuthScope{Client: kind, Permissions: []string{PermAll}})
		for _, p := range paths {
			if !verify(t, scopes, p) {
				t.Errorf("%s app password must reach %s", kind, p)
			}
		}
	}
	unknown := appAuthScopes(t, AppAuthScope{Client: "printer", Permissions: []string{PermAll}})
	if verify(t, unknown, "/remote.php/webdav") {
		t.Error("an app password of an unknown kind must reach no path")
	}
}

func TestAppAuthClientRules(t *testing.T) {
	cases := []struct {
		kind           string
		basic, bearer  bool
		ua             string
		uaMatch        bool
		strictByDefult bool
	}{
		{ClientSync, true, false, "Mozilla/5.0 (Linux) mirall/3.16.0 (Nextcloud)", true, true},
		{ClientSync, true, false, "curl/8.0", false, true},
		{ClientCLI, false, true, "cernbox-cli/1.2.0", true, true},
		{ClientCLI, false, true, "curl/8.0", false, true},
		{ClientWebDAV, true, true, "rclone/v1.66", true, false},
		{ClientMCP, false, true, "claude-code/2.0.1", true, false},
	}
	for _, c := range cases {
		r, ok := GetClientRule(c.kind)
		if !ok {
			t.Fatalf("no rule for %s", c.kind)
		}
		if r.PresentationAllowed(PresentationBasic) != c.basic || r.PresentationAllowed(PresentationBearer) != c.bearer {
			t.Errorf("%s: presentations %v", c.kind, r.Presentations)
		}
		if r.UserAgentMatches(c.ua) != c.uaMatch {
			t.Errorf("%s: user agent %q match = %v", c.kind, c.ua, !c.uaMatch)
		}
		if r.Strict != c.strictByDefult {
			t.Errorf("%s: strict = %v", c.kind, r.Strict)
		}
	}
}

func TestClientPathRule(t *testing.T) {
	cases := []struct {
		kind, path string
		allowed    bool
	}{
		{ClientMCP, "/mcp", true},
		{ClientMCP, "/mcp/", true},
		{ClientMCP, "/data/simple/eos/user/e/einstein/a.txt", true},
		{ClientMCP, "/data", true},
		{ClientMCP, "/mcpx", false},
		{ClientMCP, "/database", false},
		{ClientMCP, "/remote.php/webdav/a", false},
		{ClientMCP, "/ocs/v2.php/cloud/user", false},
		{ClientMCP, "/mcp/../remote.php/webdav", false},
		{ClientMCP, "/", false},
		{ClientSync, "/remote.php/webdav/a", true},
		{ClientSync, "/mcp", true},
		{ClientCLI, "/ocs/v2.php/cloud/user", true},
		{ClientWebDAV, "/anything", true},
	}
	for _, c := range cases {
		r, _ := GetClientRule(c.kind)
		if got := r.PathAllowed(c.path); got != c.allowed {
			t.Errorf("%s at %s: allowed = %v", c.kind, c.path, got)
		}
	}
	if !(ClientRule{Paths: []string{"/"}}).PathAllowed("/remote.php") {
		t.Error("a root prefix allows every path")
	}
	if (ClientRule{Paths: []string{}}).PathAllowed("/mcp") {
		t.Error("an empty, non-nil list allows no path")
	}
}

func TestAppAuthRoots(t *testing.T) {
	scopes := appAuthScopes(t, AppAuthScope{Client: ClientWebDAV, Permissions: ScriptPermissions(), Roots: []string{testRoot, "/eos/project/c/cernbox"}})
	inside := &provider.Reference{Path: testRoot + "/figures"}
	cases := []struct {
		name string
		req  any
		want bool
	}{
		{"the root itself", &provider.StatRequest{Ref: &provider.Reference{Path: testRoot}}, true},
		{"under the root", &provider.StatRequest{Ref: inRoot}, true},
		{"under the second root", &provider.ListContainerRequest{Ref: &provider.Reference{Path: "/eos/project/c/cernbox/docs"}}, true},
		{"outside", &provider.StatRequest{Ref: outRoot}, false},
		{"parent of the root", &provider.ListContainerRequest{Ref: &provider.Reference{Path: "/eos/user/e/einstein"}}, false},
		{"sibling sharing the prefix", &provider.StatRequest{Ref: &provider.Reference{Path: testRoot + "-old/a"}}, false},
		{"escaping with dot-dot", &provider.InitiateFileDownloadRequest{Ref: &provider.Reference{Path: testRoot + "/../private.tex"}}, false},
		{"relative path", &provider.StatRequest{Ref: &provider.Reference{Path: "thesis/a"}}, false},
		{"by id", &provider.StatRequest{Ref: &provider.Reference{ResourceId: byID}}, false},
		{"missing reference", &provider.StatRequest{}, false},
		{"move inside", &provider.MoveRequest{Source: inRoot, Destination: inside}, true},
		{"move out", &provider.MoveRequest{Source: inRoot, Destination: outRoot}, false},
		{"move in", &provider.MoveRequest{Source: outRoot, Destination: inside}, false},
		{"move by id", &provider.MoveRequest{Source: &provider.Reference{ResourceId: byID, Path: "./a"}, Destination: inside}, false},
		{"restore to its origin", &provider.RestoreRecycleItemRequest{Ref: inRoot, Key: "1"}, false},
		{"restore elsewhere", &provider.RestoreRecycleItemRequest{Ref: inRoot, Key: "1", RestoreRef: outRoot}, false},
		{"listing", &collaboration.ListReceivedSharesRequest{}, true},
		{"identity", &userpb.GetUserRequest{}, true},
	}
	for _, c := range cases {
		if got := verify(t, scopes, c.req); got != c.want {
			t.Errorf("%s: VerifyScope = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestAppAuthStreams(t *testing.T) {
	list := MethodResource("/cs3.gateway.v1beta1.GatewayAPI/ListContainerStream")
	cases := []struct {
		name  string
		scope AppAuthScope
		want  bool
	}{
		{"granted", AppAuthScope{Client: ClientWebDAV, Permissions: []string{PermFilesList}}, true},
		{"not granted", AppAuthScope{Client: ClientWebDAV, Permissions: []string{PermFilesRead}}, false},
		// The reference of a stream is not known when it is authorized.
		{"with roots", AppAuthScope{Client: ClientWebDAV, Permissions: []string{PermFilesList}, Roots: []string{testRoot}}, false},
	}
	for _, c := range cases {
		if got := verify(t, appAuthScopes(t, c.scope), list); got != c.want {
			t.Errorf("%s: VerifyScope = %v, want %v", c.name, got, c.want)
		}
	}
	if verify(t, appAuthScopes(t, AppAuthScope{Client: ClientSync, Permissions: []string{PermAll}}), MethodResource("/reva.control.v1beta1.Control/InvokeStream")) {
		t.Error("a control stream must be refused to any app password")
	}
}

func TestAppAuthExpiry(t *testing.T) {
	expired := appAuthScopes(t, AppAuthScope{Client: ClientWebDAV, Permissions: []string{PermAll}, ExpiresAt: time.Now().Add(-time.Minute).Unix()})
	if verify(t, expired, &provider.StatRequest{Ref: inRoot}) || verify(t, expired, "/remote.php/webdav") {
		t.Error("an expired app password must be refused")
	}
	valid := appAuthScopes(t, AppAuthScope{Client: ClientWebDAV, Permissions: []string{PermAll}, ExpiresAt: time.Now().Add(time.Hour).Unix()})
	if !verify(t, valid, &provider.StatRequest{Ref: inRoot}) || !verify(t, valid, "/remote.php/webdav") {
		t.Error("an app password before its expiry must be accepted")
	}
}

// An app password of a lightweight account must satisfy both its own rules
// and the lightweight ones.
func TestAppAuthLightweight(t *testing.T) {
	scopes := appAuthScopes(t, AppAuthScope{Client: ClientWebDAV, Permissions: []string{PermAll}, Lightweight: true})
	cases := []struct {
		name string
		req  any
		want bool
	}{
		{"allowed by both", &collaboration.ListReceivedSharesRequest{}, true},
		{"refused by the lightweight rules", &provider.DeleteRequest{Ref: inRoot}, false},
		{"path allowed by both", "/remote.php/webdav/a", true},
		{"path refused by the lightweight rules", "/archive", false},
	}
	for _, c := range cases {
		if got := verify(t, scopes, c.req); got != c.want {
			t.Errorf("%s: VerifyScope = %v, want %v", c.name, got, c.want)
		}
	}
	readOnly := appAuthScopes(t, AppAuthScope{Client: ClientWebDAV, Permissions: []string{PermFilesList}, Lightweight: true})
	if verify(t, readOnly, &collaboration.ListReceivedSharesRequest{}) {
		t.Error("the lightweight rules must not widen the permissions")
	}
}

func TestAppAuthScopeRole(t *testing.T) {
	cases := []struct {
		perms []string
		want  authpb.Role
	}{
		{ReadOnlyPermissions(), authpb.Role_ROLE_VIEWER},
		{ReadWritePermissions(), authpb.Role_ROLE_EDITOR},
		{[]string{PermPreferencesWrite}, authpb.Role_ROLE_EDITOR},
		{[]string{PermAll}, authpb.Role_ROLE_EDITOR},
	}
	for _, c := range cases {
		scopes := appAuthScopes(t, AppAuthScope{Client: ClientWebDAV, Permissions: c.perms})
		if got := scopes[appAuthScopeKey].Role; got != c.want {
			t.Errorf("%v: role %v, want %v", c.perms, got, c.want)
		}
	}
}

func TestAppAuthFromContext(t *testing.T) {
	want := AppAuthScope{PasswordID: "0d2c", Client: ClientWebDAV, Permissions: []string{PermFilesList}, Roots: []string{testRoot}, ExpiresAt: 1775000000}
	ctx := appctx.ContextSetScopes(context.Background(), appAuthScopes(t, want))
	got, ok := AppAuthFromContext(ctx)
	if !ok || !reflect.DeepEqual(*got, want) {
		t.Fatalf("AppAuthFromContext = %+v, %v", got, ok)
	}

	owner, err := AddOwnerScope(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := AppAuthFromContext(appctx.ContextSetScopes(context.Background(), owner)); ok {
		t.Error("a session without app password must have no app password scope")
	}
}

func TestAppAuthPresets(t *testing.T) {
	scripts := ScriptPermissions()
	for _, p := range []string{PermTrashPurge, PermSharesManage, PermPreferencesWrite} {
		if slices.Contains(scripts, p) {
			t.Errorf("the scripts preset must not include %s", p)
		}
	}
	for _, p := range ReadWritePermissions() {
		if !slices.Contains(scripts, p) {
			t.Errorf("the scripts preset must include %s", p)
		}
	}
	if len(ReadOnlyPermissions()) != 5 || len(ReadWritePermissions()) != 8 {
		t.Errorf("presets changed: %v, %v", ReadOnlyPermissions(), ReadWritePermissions())
	}
}

func TestAppAuthValidate(t *testing.T) {
	cases := []struct {
		name string
		s    AppAuthScope
		ok   bool
	}{
		{"read-only", AppAuthScope{Client: ClientWebDAV, Permissions: ReadOnlyPermissions()}, true},
		{"wildcard", AppAuthScope{Client: ClientSync, Permissions: []string{PermAll}}, true},
		{"roots", AppAuthScope{Client: ClientWebDAV, Permissions: ReadOnlyPermissions(), Roots: []string{testRoot}}, true},
		{"unknown client", AppAuthScope{Client: "printer", Permissions: ReadOnlyPermissions()}, false},
		{"no permission", AppAuthScope{Client: ClientWebDAV}, false},
		{"unknown permission", AppAuthScope{Client: ClientWebDAV, Permissions: []string{"files:steal"}}, false},
		{"wildcard and more", AppAuthScope{Client: ClientSync, Permissions: []string{PermAll, PermFilesList}}, false},
		{"relative root", AppAuthScope{Client: ClientWebDAV, Permissions: ReadOnlyPermissions(), Roots: []string{"thesis"}}, false},
		{"unclean root", AppAuthScope{Client: ClientWebDAV, Permissions: ReadOnlyPermissions(), Roots: []string{testRoot + "/../private"}}, false},
	}
	for _, c := range cases {
		if err := c.s.Validate(); (err == nil) != c.ok {
			t.Errorf("%s: Validate() = %v", c.name, err)
		}
	}
}
