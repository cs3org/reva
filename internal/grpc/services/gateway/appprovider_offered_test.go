package gateway

import (
	"context"
	"testing"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	ocmshare "github.com/cs3org/reva/v3/pkg/ocm/share"
)

func ocmScopeCtx(t *testing.T, appNames []string, withWebapp bool) context.Context {
	t.Helper()
	methods := []*ocm.AccessMethod{ocmshare.NewWebDavAccessMethod(nil, nil, nil)}
	if withWebapp {
		m := ocmshare.NewWebappAccessMethod(nil, nil)
		m.GetWebappOptions().AppNames = appNames
		methods = append(methods, m)
	}
	scopes, err := scope.AddCodeFlowOCMShareScope(&ocm.Share{
		Id:            &ocm.ShareId{OpaqueId: "share1"},
		Creator:       &userpb.UserId{OpaqueId: "owner"},
		AccessMethods: methods,
	}, authpb.Role_ROLE_VIEWER, nil)
	if err != nil {
		t.Fatal(err)
	}
	return appctx.ContextSetScopes(context.Background(), scopes)
}

func TestOfferedAppRestrictsRemoteOCMUsers(t *testing.T) {
	s := &svc{}

	t.Run("app offered on the share is accepted", func(t *testing.T) {
		got, err := s.offeredApp(ocmScopeCtx(t, []string{"Collabora", "CodiMD"}, true), nil, "CodiMD")
		if err != nil || got != "CodiMD" {
			t.Fatalf("got %q, %v", got, err)
		}
	})
	t.Run("app not offered on the share is refused", func(t *testing.T) {
		if _, err := s.offeredApp(ocmScopeCtx(t, []string{"Collabora"}, true), nil, "CodiMD"); err == nil {
			t.Fatal("expected an error")
		}
	})
	t.Run("a share that offers no app refuses every app", func(t *testing.T) {
		if _, err := s.offeredApp(ocmScopeCtx(t, nil, true), nil, "CodiMD"); err == nil {
			t.Fatal("expected an error")
		}
	})
	t.Run("a share without a webapp offer is not restricted", func(t *testing.T) {
		got, err := s.offeredApp(ocmScopeCtx(t, nil, false), nil, "Anything")
		if err != nil || got != "Anything" {
			t.Fatalf("got %q, %v", got, err)
		}
	})
	t.Run("a caller without OCM scopes is not restricted", func(t *testing.T) {
		got, err := s.offeredApp(context.Background(), nil, "Anything")
		if err != nil || got != "Anything" {
			t.Fatalf("got %q, %v", got, err)
		}
	})
}
