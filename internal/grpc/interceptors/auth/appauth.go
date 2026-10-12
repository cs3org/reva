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

package auth

import (
	"context"
	"path"
	"time"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"google.golang.org/grpc/metadata"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/token"
	"github.com/cs3org/reva/v3/pkg/utils/resourceid"
)

var errAppAuthDenied = errtypes.PermissionDenied("access to resource not allowed within the app password scope")

// verifyAppAuthScope decides what the appauth verifier cannot decide
// alone: the lightweight rules that need the gateway, and the roots for
// requests that reference resources or shares by id.
func verifyAppAuthScope(ctx context.Context, req any, ap *scope.AppAuthScope, user *userpb.User, mgr token.Manager,
	gw func(context.Context) (gateway.GatewayAPIClient, error)) error {
	if ap.Expired() || !ap.Permits(req) {
		return errAppAuthDenied
	}
	if !ap.Lightweight && len(ap.Roots) == 0 {
		// Nothing to resolve, so the verifier's refusal stands.
		return errAppAuthDenied
	}

	r := &appAuthResolver{ap: ap, user: user, mgr: mgr, gw: gw}
	if len(ap.Roots) > 0 {
		targets, ok := scope.AppAuthTargets(req)
		if !ok {
			return errAppAuthDenied
		}
		for _, t := range targets {
			p, err := r.path(ctx, t)
			if err != nil {
				return err
			}
			if !ap.UnderRoots(p) {
				return errAppAuthDenied
			}
		}
	}

	if ap.Lightweight {
		client, err := r.gateway(ctx)
		if err != nil {
			return err
		}
		lw, err := scope.AddLightweightAccountScope(authpb.Role_ROLE_VIEWER, nil)
		if err != nil {
			return err
		}
		if ok, _ := scope.VerifyScope(ctx, lw, req); !ok && !checkLightweightScope(ctx, req, lw, client) {
			return errAppAuthDenied
		}
	}
	return nil
}

// appAuthResolver finds the path of what a request references by id.
type appAuthResolver struct {
	ap   *scope.AppAuthScope
	user *userpb.User
	mgr  token.Manager
	gw   func(context.Context) (gateway.GatewayAPIClient, error)
	// client and rctx, the context with the token the lookups run with, are
	// set on first use.
	client gateway.GatewayAPIClient
	rctx   context.Context
}

func (r *appAuthResolver) gateway(ctx context.Context) (gateway.GatewayAPIClient, error) {
	if r.client == nil {
		client, err := r.gw(ctx)
		if err != nil {
			return nil, unverifiedError{err}
		}
		r.client = client
	}
	return r.client, nil
}

func (r *appAuthResolver) path(ctx context.Context, t scope.AppAuthTarget) (string, error) {
	switch {
	case t.Ref != nil && t.Ref.ResourceId == nil:
		if !path.IsAbs(t.Ref.Path) {
			return "", errAppAuthDenied
		}
		return t.Ref.Path, nil
	case t.Ref != nil:
		base, err := r.resourcePath(ctx, t.Ref.ResourceId)
		if err != nil {
			return "", err
		}
		// Relative to the resource, as the storage providers resolve it.
		return path.Join(base, t.Ref.Path), nil
	}

	key, lookup := r.shareLookup(t)
	if lookup == nil {
		return "", errAppAuthDenied
	}
	return r.cached(key, func() (string, error) {
		client, rctx, err := r.lookupContext(ctx)
		if err != nil {
			return "", err
		}
		id, err := lookup(rctx, client)
		if err != nil {
			return "", err
		}
		return r.resourcePath(ctx, id)
	})
}

// shareLookup returns the cache key of a share and a lookup of its resource.
func (r *appAuthResolver) shareLookup(t scope.AppAuthTarget) (string, func(context.Context, gateway.GatewayAPIClient) (*provider.ResourceId, error)) {
	switch {
	case t.Share.GetId() != nil:
		return "share" + scopeDelimiter + t.Share.GetId().GetOpaqueId(), func(ctx context.Context, client gateway.GatewayAPIClient) (*provider.ResourceId, error) {
			res, err := client.GetShare(ctx, &collaboration.GetShareRequest{Ref: t.Share})
			if err := lookupError(res, err); err != nil {
				return nil, err
			}
			return res.GetShare().GetResourceId(), nil
		}
	case t.ReceivedShare.GetId() != nil:
		return "receivedshare" + scopeDelimiter + t.ReceivedShare.GetId().GetOpaqueId(), func(ctx context.Context, client gateway.GatewayAPIClient) (*provider.ResourceId, error) {
			res, err := client.GetReceivedShare(ctx, &collaboration.GetReceivedShareRequest{Ref: t.ReceivedShare})
			if err := lookupError(res, err); err != nil {
				return nil, err
			}
			return res.GetShare().GetShare().GetResourceId(), nil
		}
	case t.PublicShare != nil:
		key := "publicshare" + scopeDelimiter + t.PublicShare.GetId().GetOpaqueId()
		if t.PublicShare.GetId() == nil {
			key = "publiclink" + scopeDelimiter + t.PublicShare.GetToken()
		}
		return key, func(ctx context.Context, client gateway.GatewayAPIClient) (*provider.ResourceId, error) {
			res, err := client.GetPublicShare(ctx, &link.GetPublicShareRequest{Ref: t.PublicShare})
			if err := lookupError(res, err); err != nil {
				return nil, err
			}
			return res.GetShare().GetResourceId(), nil
		}
	}
	return "", nil
}

func (r *appAuthResolver) resourcePath(ctx context.Context, id *provider.ResourceId) (string, error) {
	if id == nil {
		return "", errAppAuthDenied
	}
	return r.cached("id"+scopeDelimiter+resourceid.OwnCloudResourceIDWrap(id), func() (string, error) {
		client, rctx, err := r.lookupContext(ctx)
		if err != nil {
			return "", err
		}
		res, err := client.GetPath(rctx, &provider.GetPathRequest{ResourceId: id})
		if err := lookupError(res, err); err != nil {
			return "", err
		}
		if !path.IsAbs(res.GetPath()) {
			return "", errAppAuthDenied
		}
		return res.GetPath(), nil
	})
}

// cached keeps what was resolved per app password, for as long as the other
// scope expansions are kept.
func (r *appAuthResolver) cached(key string, resolve func() (string, error)) (string, error) {
	key = "appauth" + scopeDelimiter + r.ap.PasswordID + scopeDelimiter + key
	if v, err := scopeExpansionCache.Get(key); err == nil {
		if p, ok := v.(string); ok {
			return p, nil
		}
	}
	p, err := resolve()
	if err != nil {
		return "", err
	}
	_ = scopeExpansionCache.SetWithExpire(key, p, scopeCacheExpiration*time.Second)
	return p, nil
}

// lookupContext returns the gateway and a context whose token lets the
// lookups run without recursing into this check: the same user and app
// password, allowed to list files and shares, without roots. The token is
// never handed out.
func (r *appAuthResolver) lookupContext(ctx context.Context) (gateway.GatewayAPIClient, context.Context, error) {
	client, err := r.gateway(ctx)
	if err != nil {
		return nil, nil, err
	}
	if r.rctx != nil {
		return client, r.rctx, nil
	}
	scopes, err := scope.AddAppAuthScope(&scope.AppAuthScope{
		PasswordID:  r.ap.PasswordID,
		Client:      r.ap.Client,
		Permissions: []string{scope.PermFilesList, scope.PermSharesList},
		ExpiresAt:   r.ap.ExpiresAt,
	}, nil)
	if err != nil {
		return nil, nil, err
	}
	tkn, err := r.mgr.MintToken(ctx, r.user, scopes)
	if err != nil {
		return nil, nil, err
	}
	// Replace the caller's token, which the outgoing metadata already carries.
	md, ok := metadata.FromOutgoingContext(ctx)
	if ok {
		md = md.Copy()
	} else {
		md = metadata.MD{}
	}
	md.Set(appctx.TokenHeader, tkn)
	r.rctx = metadata.NewOutgoingContext(appctx.ContextSetToken(ctx, tkn), md)
	return client, r.rctx, nil
}

func lookupError(res interface{ GetStatus() *rpc.Status }, err error) error {
	if err != nil {
		return unverifiedError{err}
	}
	if res.GetStatus().GetCode() != rpc.Code_CODE_OK {
		return errAppAuthDenied
	}
	return nil
}
