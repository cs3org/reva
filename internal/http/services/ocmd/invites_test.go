package ocmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/owncloud/reva/v2/pkg/rgrpc/todo/pool"
	cs3mocks "github.com/owncloud/reva/v2/tests/cs3mocks/mocks"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
)

// newInvitesTestHandler wires gc into an invitesHandler without calling init().
func newInvitesTestHandler(gc *cs3mocks.GatewayAPIClient) *invitesHandler {
	pool.RemoveSelector("GatewaySelector" + "any")
	sel := pool.GetSelector[gateway.GatewayAPIClient](
		"GatewaySelector",
		"any",
		func(cc grpc.ClientConnInterface) gateway.GatewayAPIClient { return gc },
	)
	return &invitesHandler{gatewaySelector: sel}
}

// validAcceptInviteBody returns a well-formed acceptInviteRequest JSON body for the given recipientProvider.
func validAcceptInviteBody(recipientProvider string) string {
	return `{
		"token": "invite-token-123",
		"userID": "alice",
		"recipientProvider": "` + recipientProvider + `",
		"name": "Alice",
		"email": "alice@` + recipientProvider + `"
	}`
}

// doInviteRequest fires a POST /invite-accepted with the given body and returns the recorder.
func doInviteRequest(t *testing.T, h *invitesHandler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/invite-accepted", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "1.2.3.4:1234"
	w := httptest.NewRecorder()
	h.AcceptInvite(w, req)
	return w
}

// TestAcceptInvite_ProviderGate_OCISDEV756 regression test.
func TestAcceptInvite_ProviderGate_OCISDEV756(t *testing.T) {
	const trustedDomain = "trusted-partner.example"

	t.Run("untrusted recipientProvider must be rejected HTTP 401 (OCISDEV-756)", func(t *testing.T) {
		gc := &cs3mocks.GatewayAPIClient{}
		setupInfoByDomain(gc, t, trustedDomain)
		gc.On("AcceptInvite", mock.Anything, mock.Anything).Maybe().Return(
			&invitepb.AcceptInviteResponse{
				Status: &rpc.Status{Code: rpc.Code_CODE_OK},
				UserId: &userpb.UserId{OpaqueId: "alice"},
			}, nil)

		w := doInviteRequest(t, newInvitesTestHandler(gc), validAcceptInviteBody("unknown.example"))

		if w.Code != http.StatusUnauthorized {
			t.Errorf("unknown recipientProvider: got HTTP %d, want 401", w.Code)
		}
		gc.AssertNotCalled(t, "AcceptInvite", mock.Anything, mock.Anything)
	})

	t.Run("trusted recipientProvider proceeds to AcceptInvite", func(t *testing.T) {
		gc := &cs3mocks.GatewayAPIClient{}
		setupInfoByDomain(gc, t, trustedDomain)
		gc.On("IsProviderAllowed", mock.Anything, mock.Anything).Return(
			&ocmprovider.IsProviderAllowedResponse{
				Status: &rpc.Status{Code: rpc.Code_CODE_OK},
			}, nil)
		gc.On("AcceptInvite", mock.Anything, mock.Anything).Return(
			&invitepb.AcceptInviteResponse{
				Status:      &rpc.Status{Code: rpc.Code_CODE_OK},
				UserId:      &userpb.UserId{OpaqueId: "trusted-user"},
				DisplayName: "Trusted User",
				Email:       "user@trusted-partner.example",
			}, nil)

		w := doInviteRequest(t, newInvitesTestHandler(gc), validAcceptInviteBody(trustedDomain))

		gc.AssertCalled(t, "AcceptInvite", mock.Anything, mock.Anything)
		_ = w
	})
}
