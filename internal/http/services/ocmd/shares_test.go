package ocmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocmcore "github.com/cs3org/go-cs3apis/cs3/ocm/core/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	jsonauth "github.com/owncloud/reva/v2/pkg/ocm/provider/authorizer/json"
	"github.com/owncloud/reva/v2/pkg/rgrpc/todo/pool"
	cs3mocks "github.com/owncloud/reva/v2/tests/cs3mocks/mocks"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
)

// validShareBody returns a well-formed createShareRequest JSON body for the given sender.
func validShareBody(sender string) string {
	return `{
		"shareWith": "einstein@localhost:9200",
		"name": "test-share.pdf",
		"providerId": "provider-123",
		"owner": "` + sender + `",
		"sender": "` + sender + `",
		"shareType": "user",
		"resourceType": "file",
		"protocol": {
			"webdav": {
				"sharedSecret": "shared-secret-value",
				"permissions": ["read"],
				"uri": "https://unknown.example/dav/files"
			}
		}
	}`
}

// newTestHandler wires gc into a sharesHandler without calling init().
func newTestHandler(gc *cs3mocks.GatewayAPIClient) *sharesHandler {
	pool.RemoveSelector("GatewaySelector" + "any")
	sel := pool.GetSelector[gateway.GatewayAPIClient](
		"GatewaySelector",
		"any",
		func(cc grpc.ClientConnInterface) gateway.GatewayAPIClient { return gc },
	)
	return &sharesHandler{gatewaySelector: sel, allowHTTP: true}
}

// setupInfoByDomain wires the real json.Authorizer for trustedDomain into gc's GetInfoByDomain mock.
func setupInfoByDomain(gc *cs3mocks.GatewayAPIClient, t *testing.T, trustedDomain string) {
	t.Helper()
	f := filepath.Join(t.TempDir(), "providers.json")
	_ = os.WriteFile(f, []byte(`[{"domain":"`+trustedDomain+`","services":[{"endpoint":{"type":{"name":"OCM"},"path":"https://`+trustedDomain+`/ocm/"},"host":"`+trustedDomain+`"}]}]`), 0600)
	auth, err := jsonauth.New(map[string]any{"providers": f})
	if err != nil {
		t.Fatalf("jsonauth.New: %v", err)
	}
	gc.EXPECT().GetInfoByDomain(mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, req *ocmprovider.GetInfoByDomainRequest, _ ...grpc.CallOption) (*ocmprovider.GetInfoByDomainResponse, error) {
			info, err := auth.GetInfoByDomain(ctx, req.Domain)
			if err != nil {
				return &ocmprovider.GetInfoByDomainResponse{
					Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND, Message: err.Error()},
				}, nil
			}
			return &ocmprovider.GetInfoByDomainResponse{
				Status:       &rpc.Status{Code: rpc.Code_CODE_OK},
				ProviderInfo: info,
			}, nil
		})
}

// doShareRequest fires a POST /shares with the given body and returns the recorder.
func doShareRequest(t *testing.T, h *sharesHandler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/shares", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "1.2.3.4:1234"
	w := httptest.NewRecorder()
	h.CreateShare(w, req)
	return w
}

// TestCreateShare_ProviderGate_OCISDEV756 verifies unknown providers are rejected before share creation.
func TestCreateShare_ProviderGate_OCISDEV756(t *testing.T) {
	const trustedDomain = "trusted-partner.example"

	t.Run("unknown provider must be rejected HTTP 401", func(t *testing.T) {
		gc := &cs3mocks.GatewayAPIClient{}
		setupInfoByDomain(gc, t, trustedDomain)
		gc.On("GetUser", mock.Anything, mock.Anything).Maybe().Return(
			&userpb.GetUserResponse{
				Status: &rpc.Status{Code: rpc.Code_CODE_OK},
				User:   &userpb.User{Id: &userpb.UserId{OpaqueId: "einstein"}},
			}, nil)
		gc.On("CreateOCMCoreShare", mock.Anything, mock.Anything).Maybe().Return(
			&ocmcore.CreateOCMCoreShareResponse{
				Status: &rpc.Status{Code: rpc.Code_CODE_OK},
				Id:     "share-id",
			}, nil)

		w := doShareRequest(t, newTestHandler(gc), validShareBody("alice@unknown.example"))

		if w.Code != http.StatusUnauthorized {
			t.Errorf("unknown provider: got HTTP %d, want 401", w.Code)
		}
		gc.AssertNotCalled(t, "CreateOCMCoreShare", mock.Anything, mock.Anything)
	})

	t.Run("trusted domain but no invite relationship must be rejected HTTP 401", func(t *testing.T) {
		gc := &cs3mocks.GatewayAPIClient{}
		setupInfoByDomain(gc, t, trustedDomain)
		gc.On("IsProviderAllowed", mock.Anything, mock.Anything).Return(
			&ocmprovider.IsProviderAllowedResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}}, nil)
		gc.On("GetUser", mock.Anything, mock.Anything).Return(
			&userpb.GetUserResponse{
				Status: &rpc.Status{Code: rpc.Code_CODE_OK},
				User:   &userpb.User{Id: &userpb.UserId{OpaqueId: "einstein"}},
			}, nil)
		gc.On("GetAcceptedUser", mock.Anything, mock.Anything).Return(
			&invitepb.GetAcceptedUserResponse{
				Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND, Message: "no invite relationship"},
			}, nil)
		gc.On("CreateOCMCoreShare", mock.Anything, mock.Anything).Maybe().Return(
			&ocmcore.CreateOCMCoreShareResponse{
				Status: &rpc.Status{Code: rpc.Code_CODE_OK},
				Id:     "share-id",
			}, nil)

		w := doShareRequest(t, newTestHandler(gc), validShareBody("alice@"+trustedDomain))

		if w.Code != http.StatusUnauthorized {
			t.Errorf("no invite relationship: got HTTP %d, want 401", w.Code)
		}
		gc.AssertNotCalled(t, "CreateOCMCoreShare", mock.Anything, mock.Anything)
	})

	t.Run("trusted provider with invite relationship proceeds to persistence", func(t *testing.T) {
		gc := &cs3mocks.GatewayAPIClient{}
		setupInfoByDomain(gc, t, trustedDomain)
		gc.On("IsProviderAllowed", mock.Anything, mock.Anything).Return(
			&ocmprovider.IsProviderAllowedResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}}, nil)
		gc.On("GetUser", mock.Anything, mock.Anything).Return(
			&userpb.GetUserResponse{
				Status: &rpc.Status{Code: rpc.Code_CODE_OK},
				User:   &userpb.User{Id: &userpb.UserId{OpaqueId: "einstein"}},
			}, nil)
		gc.On("GetAcceptedUser", mock.Anything, mock.Anything).Return(
			&invitepb.GetAcceptedUserResponse{
				Status:     &rpc.Status{Code: rpc.Code_CODE_OK},
				RemoteUser: &userpb.User{Id: &userpb.UserId{OpaqueId: "alice", Idp: trustedDomain}},
			}, nil)
		gc.On("CreateOCMCoreShare", mock.Anything, mock.Anything).Return(
			&ocmcore.CreateOCMCoreShareResponse{
				Status: &rpc.Status{Code: rpc.Code_CODE_OK},
				Id:     "share-id",
			}, nil)

		w := doShareRequest(t, newTestHandler(gc), validShareBody("alice@"+trustedDomain))

		gc.AssertCalled(t, "CreateOCMCoreShare", mock.Anything, mock.Anything)
		_ = w
	})
}
