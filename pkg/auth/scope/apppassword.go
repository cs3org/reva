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
	"encoding/json"
	"fmt"
	"path"
	"reflect"
	"slices"
	"strings"
	"time"

	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	appregistry "github.com/cs3org/go-cs3apis/cs3/app/registry/v1beta1"
	appauthpb "github.com/cs3org/go-cs3apis/cs3/auth/applications/v1beta1"
	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	authregistry "github.com/cs3org/go-cs3apis/cs3/auth/registry/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	tenantpb "github.com/cs3org/go-cs3apis/cs3/identity/tenant/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	labels "github.com/cs3org/go-cs3apis/cs3/labels/v1beta1"
	ocmcore "github.com/cs3org/go-cs3apis/cs3/ocm/core/v1beta1"
	ocmincoming "github.com/cs3org/go-cs3apis/cs3/ocm/incoming/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	permissions "github.com/cs3org/go-cs3apis/cs3/permissions/v1beta1"
	prefpb "github.com/cs3org/go-cs3apis/cs3/preferences/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registry "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	datatx "github.com/cs3org/go-cs3apis/cs3/tx/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

const appPasswordScopeKey = "apppassword"

// App password permissions, the catalogue of operations an app password can
// be granted.
const (
	PermFilesList        = "files:list"
	PermFilesRead        = "files:read"
	PermFilesWrite       = "files:write"
	PermFilesMove        = "files:move"
	PermFilesDelete      = "files:delete"
	PermVersionsList     = "versions:list"
	PermVersionsRestore  = "versions:restore"
	PermTrashList        = "trash:list"
	PermTrashRestore     = "trash:restore"
	PermTrashPurge       = "trash:purge"
	PermSharesList       = "shares:list"
	PermSharesManage     = "shares:manage"
	PermDirectorySearch  = "directory:search"
	PermLabelsRead       = "labels:read"
	PermLabelsWrite      = "labels:write"
	PermAppsOpen         = "apps:open"
	PermPreferencesWrite = "preferences:write"

	// PermAll allows every request outside the never list. Only the login
	// flow mints it, for sync clients.
	PermAll = "*"
)

// AppPasswordScope is the resource of the apppassword scope.
type AppPasswordScope struct {
	PasswordID  string   `json:"password_id"`
	Client      string   `json:"client"`
	Permissions []string `json:"permissions"`
	Roots       []string `json:"roots,omitempty"`
	ExpiresAt   int64    `json:"expires_at,omitempty"`
	Lightweight bool     `json:"lightweight,omitempty"`
}

// catalogue maps each permission to the requests it allows. Besides the
// gateway requests, it lists the provider requests the gateway sends with the
// caller's token while serving them, since the same scope is verified again in
// every provider.
var catalogue = []struct {
	permission string
	write      bool
	requests   []any
}{
	{PermFilesList, false, []any{
		&provider.StatRequest{}, &provider.ListContainerRequest{}, &provider.ListContainerStreamRequest{},
		&provider.GetPathRequest{}, &provider.GetQuotaRequest{}, &gateway.GetQuotaRequest{}, &provider.ListStorageSpacesRequest{},
		&provider.GetHomeRequest{}, &provider.GetLockRequest{},
		&registry.GetStorageProvidersRequest{}, &registry.ListStorageProvidersRequest{}, &registry.GetHomeRequest{},
	}},
	{PermFilesRead, false, []any{&provider.InitiateFileDownloadRequest{}}},
	{PermFilesWrite, true, []any{
		&provider.InitiateFileUploadRequest{}, &provider.TouchFileRequest{}, &provider.CreateContainerRequest{},
		&provider.SetArbitraryMetadataRequest{}, &provider.UnsetArbitraryMetadataRequest{},
		&provider.SetLockRequest{}, &provider.RefreshLockRequest{}, &provider.UnlockRequest{},
		&provider.CreateReferenceRequest{}, &provider.CreateSymlinkRequest{},
	}},
	{PermFilesMove, true, []any{&provider.MoveRequest{}}},
	{PermFilesDelete, true, []any{&provider.DeleteRequest{}}},
	{PermVersionsList, false, []any{&provider.ListFileVersionsRequest{}}},
	{PermVersionsRestore, true, []any{&provider.RestoreFileVersionRequest{}}},
	{PermTrashList, false, []any{&provider.ListRecycleRequest{}, &provider.ListRecycleStreamRequest{}}},
	{PermTrashRestore, true, []any{&provider.RestoreRecycleItemRequest{}}},
	{PermTrashPurge, true, []any{&provider.PurgeRecycleRequest{}}},
	{PermSharesList, false, []any{
		&collaboration.ListSharesRequest{}, &collaboration.GetShareRequest{},
		&collaboration.ListReceivedSharesRequest{}, &collaboration.GetReceivedShareRequest{},
		&link.ListPublicSharesRequest{}, &link.GetPublicShareRequest{}, &link.GetPublicShareByTokenRequest{},
		&ocm.ListOCMSharesRequest{}, &ocm.GetOCMShareRequest{},
		&ocm.ListReceivedOCMSharesRequest{}, &ocm.GetReceivedOCMShareRequest{},
		&provider.ListGrantsRequest{},
	}},
	{PermSharesManage, true, []any{
		&collaboration.CreateShareRequest{}, &collaboration.UpdateShareRequest{}, &collaboration.RemoveShareRequest{},
		&collaboration.UpdateReceivedShareRequest{},
		&link.CreatePublicShareRequest{}, &link.UpdatePublicShareRequest{}, &link.RemovePublicShareRequest{},
		&provider.AddGrantRequest{}, &provider.UpdateGrantRequest{}, &provider.RemoveGrantRequest{}, &provider.DenyGrantRequest{},
	}},
	{PermDirectorySearch, false, []any{
		&userpb.FindUsersRequest{}, &grouppb.FindGroupsRequest{}, &grouppb.GetMembersRequest{},
		// The gateway's FindUsers also searches the accepted OCM users.
		&invitepb.FindAcceptedUsersRequest{}, &invitepb.GetAcceptedUserRequest{},
	}},
	{PermLabelsRead, false, []any{&labels.ListLabelsRequest{}, &labels.ListResourcesForLabelRequest{}}},
	{PermLabelsWrite, true, []any{&labels.AddLabelRequest{}, &labels.RemoveLabelRequest{}}},
	{PermAppsOpen, false, []any{
		&gateway.OpenInAppRequest{}, &appprovider.OpenInAppRequest{},
		&appregistry.GetAppProvidersRequest{}, &appregistry.ListAppProvidersRequest{},
		&appregistry.ListSupportedMimeTypesRequest{}, &appregistry.GetDefaultAppProviderForMimeTypeRequest{},
	}},
	{PermPreferencesWrite, true, []any{&prefpb.SetKeyRequest{}}},
}

// alwaysAllowed is identity plumbing with no data access.
var alwaysAllowed = typeSet(
	&userpb.GetUserRequest{}, &userpb.GetUserByClaimRequest{}, &userpb.GetUserGroupsRequest{},
	&grouppb.GetGroupRequest{}, &grouppb.GetGroupByClaimRequest{}, &grouppb.HasMemberRequest{},
	&prefpb.GetKeyRequest{},
	&tenantpb.GetTenantRequest{}, &tenantpb.GetTenantByClaimRequest{},
	&gateway.WhoAmIRequest{}, &permissions.CheckPermissionRequest{},
	&authregistry.GetAuthProvidersRequest{}, &authregistry.ListAuthProvidersRequest{},
)

// neverAllowed holds what no app password may do, not even with PermAll. The
// Admin API is excluded as well, through isAdminResource.
var neverAllowed = typeSet(
	// Managing app passwords from an app password would let a leaked one widen
	// itself; minting tokens and stepping up to admin likewise.
	&appauthpb.GenerateAppPasswordRequest{}, &appauthpb.ListAppPasswordsRequest{},
	&appauthpb.InvalidateAppPasswordRequest{}, &appauthpb.GetAppPasswordRequest{},
	&gateway.AuthenticateRequest{}, &authpb.AuthenticateRequest{},
	&adminpb.RequestAdminRequest{},

	&provider.CreateStorageSpaceRequest{}, &provider.UpdateStorageSpaceRequest{}, &provider.DeleteStorageSpaceRequest{},
	&provider.CreateHomeRequest{}, &provider.SetImmutableRequest{}, &provider.UnsetImmutableRequest{},

	// OCM invites and OCM share management.
	&ocm.CreateOCMShareRequest{}, &ocm.UpdateOCMShareRequest{}, &ocm.RemoveOCMShareRequest{},
	&ocm.UpdateReceivedOCMShareRequest{}, &ocm.GetOCMShareByTokenRequest{},
	&invitepb.GenerateInviteTokenRequest{}, &invitepb.ListInviteTokensRequest{}, &invitepb.ForwardInviteRequest{},
	&invitepb.AcceptInviteRequest{}, &invitepb.DeleteAcceptedUserRequest{},
	&ocmprovider.IsProviderAllowedRequest{}, &ocmprovider.GetInfoByDomainRequest{}, &ocmprovider.ListAllProvidersRequest{},
	&ocmincoming.CreateOCMIncomingShareRequest{}, &ocmincoming.UpdateOCMIncomingShareRequest{},
	&ocmincoming.DeleteOCMIncomingShareRequest{},
	&ocmcore.CreateOCMCoreShareRequest{}, &ocmcore.UpdateOCMCoreShareRequest{}, &ocmcore.DeleteOCMCoreShareRequest{},

	&datatx.CreateTransferRequest{}, &datatx.GetTransferStatusRequest{}, &datatx.CancelTransferRequest{},
	&datatx.ListTransfersRequest{}, &datatx.RetryTransferRequest{},

	// Daemon plumbing.
	&gateway.PublishEventRequest{}, &appregistry.AddAppProviderRequest{},
	&appregistry.SetDefaultAppProviderForMimeTypeRequest{},
)

var (
	requestPermission = map[reflect.Type]string{}
	writePermission   = map[string]bool{}
)

func init() {
	for _, c := range catalogue {
		writePermission[c.permission] = c.write
		for _, r := range c.requests {
			requestPermission[reflect.TypeOf(r)] = c.permission
		}
	}
}

func typeSet(requests ...any) map[reflect.Type]bool {
	s := make(map[reflect.Type]bool, len(requests))
	for _, r := range requests {
		s[reflect.TypeOf(r)] = true
	}
	return s
}

// AppPasswordPermissions returns the catalogue, in display order.
func AppPasswordPermissions() []string {
	perms := make([]string, 0, len(catalogue))
	for _, c := range catalogue {
		perms = append(perms, c.permission)
	}
	return perms
}

// ReadOnlyPermissions is the default preset.
func ReadOnlyPermissions() []string {
	return []string{PermFilesList, PermFilesRead, PermSharesList, PermVersionsList, PermLabelsRead}
}

// ReadWritePermissions is the read-only preset plus writing, moving and tagging.
func ReadWritePermissions() []string {
	return append(ReadOnlyPermissions(), PermFilesWrite, PermFilesMove, PermLabelsWrite)
}

// ScriptPermissions is the catalogue except what must be granted explicitly.
func ScriptPermissions() []string {
	return slices.DeleteFunc(AppPasswordPermissions(), func(p string) bool {
		return p == PermTrashPurge || p == PermSharesManage || p == PermPreferencesWrite
	})
}

func (s *AppPasswordScope) wildcard() bool {
	return slices.Contains(s.Permissions, PermAll)
}

// Has reports whether the app password grants perm.
func (s *AppPasswordScope) Has(perm string) bool {
	return s.wildcard() || slices.Contains(s.Permissions, perm)
}

// Validate checks that the app password names a known kind of client,
// permissions from the catalogue or the wildcard alone, and clean absolute
// roots.
func (s *AppPasswordScope) Validate() error {
	if _, ok := clientRules[s.Client]; !ok {
		return errtypes.BadRequest(fmt.Sprintf("unknown client kind %q", s.Client))
	}
	if len(s.Permissions) == 0 {
		return errtypes.BadRequest("an app password needs at least one permission")
	}
	if s.wildcard() && len(s.Permissions) > 1 {
		return errtypes.BadRequest("the wildcard permission cannot be combined with others")
	}
	for _, p := range s.Permissions {
		if _, ok := writePermission[p]; !ok && p != PermAll {
			return errtypes.BadRequest(fmt.Sprintf("unknown permission %q", p))
		}
	}
	for _, r := range s.Roots {
		if !path.IsAbs(r) || path.Clean(r) != r {
			return errtypes.BadRequest(fmt.Sprintf("root %q is not a clean absolute path", r))
		}
	}
	return nil
}

// Expired reports whether the app password is past its expiry.
func (s *AppPasswordScope) Expired() bool {
	return s.ExpiresAt != 0 && time.Now().Unix() > s.ExpiresAt
}

// Permits reports whether the permissions of the app password allow req,
// regardless of its roots.
func (s *AppPasswordScope) Permits(req any) bool {
	ok, _ := s.permits(req)
	return ok
}

// permits also reports whether req is a request type the catalogue knows.
func (s *AppPasswordScope) permits(req any) (allowed, known bool) {
	typ := requestType(req)
	if typ == nil || isAdminResource(req) || neverAllowed[typ] {
		return false, true
	}
	if alwaysAllowed[typ] {
		return true, true
	}
	perm, known := requestPermission[typ]
	if !known {
		return s.wildcard(), false
	}
	return s.Has(perm), true
}

// requestType is the type of a request message, also for a streaming call,
// which the interceptor names by method.
func requestType(req any) reflect.Type {
	m, ok := req.(MethodResource)
	if !ok {
		return reflect.TypeOf(req)
	}
	service, method, ok := strings.Cut(strings.TrimPrefix(string(m), "/"), "/")
	if !ok {
		return nil
	}
	d, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(service))
	if err != nil {
		return nil
	}
	sd, ok := d.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil
	}
	md := sd.Methods().ByName(protoreflect.Name(method))
	if md == nil {
		return nil
	}
	mt, err := protoregistry.GlobalTypes.FindMessageByName(md.Input().FullName())
	if err != nil {
		return nil
	}
	return reflect.TypeOf(mt.Zero().Interface())
}

func requestName(req any) string {
	if m, ok := req.(MethodResource); ok {
		return string(m)
	}
	return fmt.Sprintf("%T", req)
}

// UnderRoots reports whether the absolute path p is inside one of the roots of
// the app password. An app password without roots covers the whole account.
func (s *AppPasswordScope) UnderRoots(p string) bool {
	if len(s.Roots) == 0 {
		return true
	}
	p = path.Clean(p)
	for _, r := range s.Roots {
		r = path.Clean(r)
		if r == "/" || p == r || strings.HasPrefix(p, r+"/") {
			return true
		}
	}
	return false
}

// AppPasswordTarget is a resource a request acts on, which must lie under the
// roots of an app password. Exactly one field is set; a share is checked
// through its resource.
type AppPasswordTarget struct {
	Ref           *provider.Reference
	Share         *collaboration.ShareReference
	ReceivedShare *collaboration.ShareReference
	PublicShare   *link.PublicShareReference
}

func (t AppPasswordTarget) absolutePath() (string, bool) {
	if t.Ref != nil && t.Ref.ResourceId == nil && path.IsAbs(t.Ref.Path) {
		return t.Ref.Path, true
	}
	return "", false
}

// AppPasswordTargets returns the resources req acts on. ok is false when req
// acts on something the roots cannot be checked against, such as a whole
// recycle bin; such a request is refused to an app password with roots.
//
// Listings and share lookups have no target: they reveal metadata only, and a
// consumer that shows them filters them against the roots.
func AppPasswordTargets(req any) ([]AppPasswordTarget, bool) {
	if alwaysAllowed[requestType(req)] {
		return nil, true
	}
	switch v := req.(type) {
	case *provider.ListStorageSpacesRequest, *provider.GetHomeRequest, *registry.ListStorageProvidersRequest,
		*registry.GetHomeRequest, *provider.ListRecycleRequest, *provider.ListRecycleStreamRequest,
		*collaboration.ListSharesRequest, *collaboration.GetShareRequest,
		*collaboration.ListReceivedSharesRequest, *collaboration.GetReceivedShareRequest,
		*link.ListPublicSharesRequest, *link.GetPublicShareRequest, *link.GetPublicShareByTokenRequest,
		*ocm.ListOCMSharesRequest, *ocm.GetOCMShareRequest,
		*ocm.ListReceivedOCMSharesRequest, *ocm.GetReceivedOCMShareRequest,
		*userpb.FindUsersRequest, *grouppb.FindGroupsRequest, *grouppb.GetMembersRequest,
		*invitepb.FindAcceptedUsersRequest, *invitepb.GetAcceptedUserRequest,
		*labels.ListLabelsRequest, *labels.ListResourcesForLabelRequest,
		*appregistry.GetAppProvidersRequest, *appregistry.ListAppProvidersRequest,
		*appregistry.ListSupportedMimeTypesRequest, *appregistry.GetDefaultAppProviderForMimeTypeRequest,
		*prefpb.SetKeyRequest:
		return nil, true

	case *provider.MoveRequest:
		return refTargets(v.GetSource(), v.GetDestination())
	case *provider.GetPathRequest:
		return refTargets(&provider.Reference{ResourceId: v.GetResourceId()})
	case *provider.RestoreRecycleItemRequest:
		// Without a destination the item goes back to where it was deleted
		// from, which the request does not say.
		return refTargets(v.GetRestoreRef())

	case *collaboration.CreateShareRequest:
		return refTargets(&provider.Reference{ResourceId: v.GetResourceInfo().GetId()})
	case *collaboration.UpdateShareRequest:
		var targets []AppPasswordTarget
		if v.GetRef() != nil {
			t, ok := shareTargets(v.GetRef())
			if !ok {
				return nil, false
			}
			targets = t
		}
		if id := v.GetShare().GetId(); id != nil {
			targets = append(targets, AppPasswordTarget{Share: &collaboration.ShareReference{Spec: &collaboration.ShareReference_Id{Id: id}}})
		}
		return targets, len(targets) > 0
	case *collaboration.RemoveShareRequest:
		return shareTargets(v.GetRef())
	case *collaboration.UpdateReceivedShareRequest:
		id := v.GetShare().GetShare().GetId()
		if id == nil {
			return nil, false
		}
		return []AppPasswordTarget{{ReceivedShare: &collaboration.ShareReference{Spec: &collaboration.ShareReference_Id{Id: id}}}}, true

	case *link.CreatePublicShareRequest:
		return refTargets(&provider.Reference{ResourceId: v.GetResourceInfo().GetId()})
	case *link.UpdatePublicShareRequest:
		return publicShareTargets(v.GetRef())
	case *link.RemovePublicShareRequest:
		return publicShareTargets(v.GetRef())

	case *appprovider.OpenInAppRequest:
		return refTargets(&provider.Reference{ResourceId: v.GetResourceInfo().GetId()})
	}

	// Every other request in the catalogue carries a single reference.
	if r, ok := req.(interface{ GetRef() *provider.Reference }); ok {
		switch req.(type) {
		case *provider.CreateSymlinkRequest, *provider.CreateReferenceRequest, *provider.PurgeRecycleRequest:
			// A link to elsewhere, or a key the request does not locate.
			return nil, false
		}
		return refTargets(r.GetRef())
	}
	return nil, false
}

func refTargets(refs ...*provider.Reference) ([]AppPasswordTarget, bool) {
	targets := make([]AppPasswordTarget, 0, len(refs))
	for _, r := range refs {
		if r == nil || (r.ResourceId == nil && r.Path == "") {
			return nil, false
		}
		targets = append(targets, AppPasswordTarget{Ref: r})
	}
	return targets, true
}

func shareTargets(ref *collaboration.ShareReference) ([]AppPasswordTarget, bool) {
	switch {
	case ref.GetKey() != nil:
		return refTargets(&provider.Reference{ResourceId: ref.GetKey().GetResourceId()})
	case ref.GetId() != nil:
		return []AppPasswordTarget{{Share: ref}}, true
	}
	return nil, false
}

func publicShareTargets(ref *link.PublicShareReference) ([]AppPasswordTarget, bool) {
	if ref.GetId() == nil && ref.GetToken() == "" {
		return nil, false
	}
	return []AppPasswordTarget{{PublicShare: ref}}, true
}

func appPasswordScope(ctx context.Context, sc *authpb.Scope, resource any, log *zerolog.Logger) (bool, error) {
	s, err := parseAppPasswordScope(sc)
	if err != nil {
		return false, err
	}
	if !s.verify(resource, log) {
		return false, nil
	}
	// An app password of a lightweight account can never do more than the
	// account's own sessions, so both rules must pass.
	if s.Lightweight {
		return lightweightAccountScope(ctx, sc, resource, log)
	}
	return true, nil
}

func (s *AppPasswordScope) verify(resource any, log *zerolog.Logger) bool {
	if s.Expired() {
		return false
	}
	if p, ok := resource.(string); ok {
		// An unknown kind has no rule and so no allowed path.
		return clientRules[s.Client].PathAllowed(p)
	}

	allowed, known := s.permits(resource)
	if !known && !allowed {
		log.Warn().Str("request_type", requestName(resource)).Str("password_id", s.PasswordID).
			Msg("apppassword: request type not in catalogue")
	}
	if !allowed || len(s.Roots) == 0 {
		return allowed
	}

	targets, ok := AppPasswordTargets(resource)
	if !ok {
		return false
	}
	for _, t := range targets {
		// References by id are resolved by the auth interceptor.
		p, ok := t.absolutePath()
		if !ok || !s.UnderRoots(p) {
			return false
		}
	}
	return true
}

func parseAppPasswordScope(sc *authpb.Scope) (*AppPasswordScope, error) {
	var s AppPasswordScope
	if err := json.Unmarshal(sc.GetResource().GetValue(), &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// GetAppPasswordScope returns the app password scope in scopes, if any.
func GetAppPasswordScope(scopes map[string]*authpb.Scope) (*AppPasswordScope, bool) {
	sc, ok := scopes[appPasswordScopeKey]
	if !ok {
		return nil, false
	}
	s, err := parseAppPasswordScope(sc)
	if err != nil {
		return nil, false
	}
	return s, true
}

// AppPasswordFromContext returns the app password scope of the request, when
// it was authenticated with an app password.
func AppPasswordFromContext(ctx context.Context) (*AppPasswordScope, bool) {
	scopes, ok := appctx.ContextGetScopes(ctx)
	if !ok {
		return nil, false
	}
	return GetAppPasswordScope(scopes)
}

// AddAppPasswordScope adds the scope of an app password.
func AddAppPasswordScope(s *AppPasswordScope, scopes map[string]*authpb.Scope) (map[string]*authpb.Scope, error) {
	val, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	role := authpb.Role_ROLE_VIEWER
	if slices.ContainsFunc(s.Permissions, func(p string) bool { return p == PermAll || writePermission[p] }) {
		role = authpb.Role_ROLE_EDITOR
	}
	if scopes == nil {
		scopes = make(map[string]*authpb.Scope)
	}
	scopes[appPasswordScopeKey] = &authpb.Scope{
		Resource: &types.OpaqueEntry{
			Decoder: "json",
			Value:   val,
		},
		Role: role,
	}
	return scopes, nil
}
