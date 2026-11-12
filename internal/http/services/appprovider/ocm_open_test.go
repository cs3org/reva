package appprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	apppb "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"google.golang.org/grpc"
)

type ocmOpenGateway struct {
	gateway.GatewayAPIClient
	role    string
	open    *gateway.OpenInAppRequest
	openRes *apppb.OpenInAppResponse
}

func (g *ocmOpenGateway) Authenticate(_ context.Context, req *gateway.AuthenticateRequest, _ ...grpc.CallOption) (*gateway.AuthenticateResponse, error) {
	if req.Type != "ocmexchangedtoken" || req.ClientSecret != "exchanged.jwt.token" {
		return &gateway.AuthenticateResponse{Status: &rpc.Status{Code: rpc.Code_CODE_UNAUTHENTICATED}}, nil
	}
	return &gateway.AuthenticateResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
		Token:  "gateway-token",
		User: &userpb.User{Opaque: &typespb.Opaque{Map: map[string]*typespb.OpaqueEntry{
			"ocm-share-role": {Decoder: "plain", Value: []byte(g.role)},
			"ocm-share-id":   {Decoder: "plain", Value: []byte("share1")},
		}}},
	}, nil
}

func (g *ocmOpenGateway) OpenInApp(_ context.Context, req *gateway.OpenInAppRequest, _ ...grpc.CallOption) (*apppb.OpenInAppResponse, error) {
	g.open = req
	return g.openRes, nil
}

func postOCMOpen(t *testing.T, gw *ocmOpenGateway, target string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	stampGateway(gw)
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if i := strings.Index(target, "/ocm-open/"); i >= 0 {
		req.SetPathValue("rel", target[i+len("/ocm-open/"):])
	}
	rec := httptest.NewRecorder()
	(&svc{conf: &Config{}}).handleOCMOpen(rec, req)
	return rec
}

func okOpen() *apppb.OpenInAppResponse {
	return &apppb.OpenInAppResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
		AppUrl: &apppb.OpenInAppURL{
			AppUrl:         "https://collabora.example/edit?x=1",
			Method:         http.MethodPost,
			FormParameters: map[string]string{"access_token": "wopi-token"},
		},
	}
}

func TestHandleOCMOpenLaunchesAppForShareRole(t *testing.T) {
	for role, want := range map[string]gateway.OpenInAppRequest_ViewMode{
		"editor": gateway.OpenInAppRequest_VIEW_MODE_READ_WRITE,
		"viewer": gateway.OpenInAppRequest_VIEW_MODE_READ_ONLY,
	} {
		t.Run(role, func(t *testing.T) {
			gw := &ocmOpenGateway{role: role, openRes: okOpen()}
			rec := postOCMOpen(t, gw, "/app/ocm-open/dir/notes.md", url.Values{
				"access_token": {"exchanged.jwt.token"},
				"appname":      {"CodiMD"},
			})
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
			}
			if gw.open.GetRef().GetPath() != "/ocm/share1/dir/notes.md" || gw.open.GetApp() != "CodiMD" || gw.open.GetViewMode() != want {
				t.Fatalf("OpenInApp request = %v", gw.open)
			}
			body := rec.Body.String()
			if !strings.Contains(body, `action="https://collabora.example/edit?x=1"`) ||
				!strings.Contains(body, `name="access_token" value="wopi-token"`) {
				t.Fatalf("launch form missing: %s", body)
			}
			if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "form-action https://collabora.example;") {
				t.Fatalf("CSP = %q", csp)
			}
		})
	}
}

func TestHandleOCMOpenRejectsBadRequests(t *testing.T) {
	good := url.Values{"access_token": {"exchanged.jwt.token"}}
	tests := []struct {
		name   string
		target string
		form   url.Values
		gw     *ocmOpenGateway
		want   int
	}{
		{"missing token", "/app/ocm-open", url.Values{}, &ocmOpenGateway{}, http.StatusUnauthorized},
		{"token refused", "/app/ocm-open", url.Values{"access_token": {"other"}}, &ocmOpenGateway{}, http.StatusUnauthorized},
		{"path traversal", "/app/ocm-open/a/../b", good, &ocmOpenGateway{role: "viewer"}, http.StatusBadRequest},
		{"app not offered", "/app/ocm-open", good, &ocmOpenGateway{role: "viewer", openRes: &apppb.OpenInAppResponse{Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED}}}, http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postOCMOpen(t, tt.gw, tt.target, tt.form)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
			if strings.Contains(rec.Body.String(), "exchanged.jwt.token") {
				t.Fatal("response leaks the access token")
			}
		})
	}
}

func TestHandleOCMOpenIgnoresTokenInQuery(t *testing.T) {
	gw := &ocmOpenGateway{role: "viewer", openRes: okOpen()}
	rec := postOCMOpen(t, gw, "/app/ocm-open?access_token=exchanged.jwt.token", url.Values{})
	if rec.Code != http.StatusUnauthorized || gw.open != nil {
		t.Fatalf("status = %d, open = %v", rec.Code, gw.open)
	}
}
