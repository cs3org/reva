package utils

import (
	"context"
	"fmt"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	revactx "github.com/cs3org/reva/v2/pkg/ctx"
	"google.golang.org/grpc/metadata"
)

// Impersonate returns an authenticated reva context and the user it represents
func Impersonate(userID *user.UserId, gwc gateway.GatewayAPIClient, machineAuthAPIKey string) (context.Context, *user.User, error) {
	getUserResponse, err := gwc.GetUser(context.Background(), &user.GetUserRequest{
		UserId: userID,
	})
	if err != nil {
		return nil, nil, err
	}
	if getUserResponse.Status.Code != rpc.Code_CODE_OK {
		return nil, nil, fmt.Errorf("error getting user: %s", getUserResponse.Status.Message)
	}

	// Get auth context
	ctx := revactx.ContextSetUser(context.Background(), getUserResponse.User)
	authRes, err := gwc.Authenticate(ctx, &gateway.AuthenticateRequest{
		Type:         "machine",
		ClientId:     "userid:" + userID.OpaqueId,
		ClientSecret: machineAuthAPIKey,
	})
	if err != nil {
		return nil, nil, err
	}
	if authRes.GetStatus().GetCode() != rpc.Code_CODE_OK {
		return nil, nil, fmt.Errorf("error impersonating user: %s", authRes.Status.Message)
	}

	return metadata.AppendToOutgoingContext(ctx, revactx.TokenHeader, authRes.Token), authRes.GetUser(), nil
}
