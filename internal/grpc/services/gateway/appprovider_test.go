// Copyright 2018-2021 CERN
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

package gateway

import (
	"context"
	"testing"

	providerpb "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	auth "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	gatewayv1beta1 "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/utils"
)

type mockTokenManager struct{}

func (m *mockTokenManager) MintToken(ctx context.Context, u *user.User, scope map[string]*auth.Scope) (string, error) {
	return "mockToken", nil
}

func (m *mockTokenManager) DismantleToken(ctx context.Context, token string) (*user.User, map[string]*auth.Scope, error) {
	return nil, nil, nil
}

func TestBuildOpenInAppRequest(t *testing.T) {
	tokenmgr := &mockTokenManager{}
	t.Run("Write mode", func(t *testing.T) {
		ri := &providerv1beta1.ResourceInfo{}
		req, err := buildOpenInAppRequest(context.Background(), ri, gatewayv1beta1.OpenInAppRequest_VIEW_MODE_READ_WRITE, tokenmgr, "accessToken", nil)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if req.ViewMode != providerpb.ViewMode(gatewayv1beta1.OpenInAppRequest_VIEW_MODE_READ_WRITE) {
			t.Errorf("Unexpected view mode. Got: %v, want: %v", req.ViewMode, providerpb.ViewMode(gatewayv1beta1.OpenInAppRequest_VIEW_MODE_READ_WRITE))
		}
		if req.AccessToken != "accessToken" {
			t.Errorf("Unexpected access token. Got: %v, want: %v", req.AccessToken, "accessToken")
		}
		if req.ResourceInfo != ri {
			t.Errorf("Unexpected resource info. Got: %v, want: %v", req.ResourceInfo, ri)
		}
		if utils.ReadPlainFromOpaque(req.Opaque, "viewOnlyToken") != "" {
			t.Errorf("Unexpected opaque. Got: %v, want: %v", req.Opaque, "")
		}
	})

	t.Run("View only mode without stat permission will not mint a viewOnlyToken", func(t *testing.T) {
		ri := &providerv1beta1.ResourceInfo{}
		req, err := buildOpenInAppRequest(context.Background(), ri, gatewayv1beta1.OpenInAppRequest_VIEW_MODE_VIEW_ONLY, tokenmgr, "accessToken", nil)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if req.ViewMode != providerpb.ViewMode(gatewayv1beta1.OpenInAppRequest_VIEW_MODE_VIEW_ONLY) {
			t.Errorf("Unexpected view mode. Got: %v, want: %v", req.ViewMode, providerpb.ViewMode(gatewayv1beta1.OpenInAppRequest_VIEW_MODE_VIEW_ONLY))
		}
		if req.AccessToken != "accessToken" {
			t.Errorf("Unexpected access token. Got: %v, want: %v", req.AccessToken, "accessToken")
		}
		if req.ResourceInfo != ri {
			t.Errorf("Unexpected resource info. Got: %v, want: %v", req.ResourceInfo, ri)
		}
		if utils.ReadPlainFromOpaque(req.Opaque, "viewOnlyToken") != "" {
			t.Errorf("Unexpected opaque. Got: %v, want: %v", req.Opaque, "")
		}
	})
	t.Run("View only mode with stat permission will mint a viewOnlyToken", func(t *testing.T) {
		ri := &providerv1beta1.ResourceInfo{
			PermissionSet: &providerv1beta1.ResourcePermissions{
				Stat: true,
			},
		}
		ctx := ctxpkg.ContextSetUser(context.Background(), &user.User{Username: "a user without download permission"})
		req, err := buildOpenInAppRequest(ctx, ri, gatewayv1beta1.OpenInAppRequest_VIEW_MODE_VIEW_ONLY, tokenmgr, "accessToken", nil)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if req.ViewMode != providerpb.ViewMode(gatewayv1beta1.OpenInAppRequest_VIEW_MODE_VIEW_ONLY) {
			t.Errorf("Unexpected view mode. Got: %v, want: %v", req.ViewMode, providerpb.ViewMode(gatewayv1beta1.OpenInAppRequest_VIEW_MODE_VIEW_ONLY))
		}
		if req.AccessToken != "accessToken" {
			t.Errorf("Unexpected access token. Got: %v, want: %v", req.AccessToken, "accessToken")
		}
		if req.ResourceInfo != ri {
			t.Errorf("Unexpected resource info. Got: %v, want: %v", req.ResourceInfo, ri)
		}
		if utils.ReadPlainFromOpaque(req.Opaque, "viewOnlyToken") != "mockToken" {
			t.Errorf("Unexpected opaque. Got: %v, want: %v", req.Opaque, "mockToken")
		}
	})
}
