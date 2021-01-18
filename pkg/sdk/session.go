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

package sdk

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"

	registry "github.com/cs3org/go-cs3apis/cs3/auth/registry/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	"github.com/cs3org/reva/pkg/sdk/common"
	"github.com/cs3org/reva/pkg/sdk/common/net"
)

// Session stores information about a Reva session.
// It is also responsible for managing the Reva gateway client.
type Session struct {
	ctx    context.Context
	client gateway.GatewayAPIClient

	token string
}

func (session *Session) initSession(ctx context.Context) error {
	session.ctx = ctx

	return nil
}

// Initiate initiates the session by creating a connection to the host and preparing the gateway client.
func (session *Session) Initiate(host string, insecure bool) error {
	conn, err := session.getConnection(host, insecure)
	if err != nil {
		return fmt.Errorf("unable to establish a gRPC connection to '%v': %v", host, err)
	}
	session.client = gateway.NewGatewayAPIClient(conn)

	return nil
}

func (session *Session) getConnection(host string, insecure bool) (*grpc.ClientConn, error) {
	if insecure {
		return grpc.Dial(host, grpc.WithInsecure())
	}

	tlsconf := &tls.Config{InsecureSkipVerify: false}
	creds := credentials.NewTLS(tlsconf)
	return grpc.Dial(host, grpc.WithTransportCredentials(creds))
}

// GetLoginMethods returns a list of all available login methods supported by the Reva instance.
func (session *Session) GetLoginMethods() ([]string, error) {
	req := &registry.ListAuthProvidersRequest{}
	res, err := session.client.ListAuthProviders(session.ctx, req)
	if err := net.CheckRPCInvocation("listing authorization providers", res, err); err != nil {
		return []string{}, err
	}

	return res.Types, nil
}

// Login logs into Reva using the specified method and user credentials.
func (session *Session) Login(method string, username string, password string) error {
	req := &gateway.AuthenticateRequest{
		Type:         method,
		ClientId:     username,
		ClientSecret: password,
	}
	res, err := session.client.Authenticate(session.ctx, req)
	if err := net.CheckRPCInvocation("authenticating", res, err); err != nil {
		return err
	}

	if res.Token == "" {
		return fmt.Errorf("invalid token received: %q", res.Token)
	}
	session.token = res.Token

	// Now that we have a valid token, we can append this to our context
	session.ctx = context.WithValue(session.ctx, net.AccessTokenIndex, session.token)
	session.ctx = metadata.AppendToOutgoingContext(session.ctx, net.AccessTokenName, session.token)

	return nil
}

// BasicLogin tries to log into Reva using basic authentication.
// Before the actual login attempt, the method verifies that the Reva instance does support the "basic" login method.
func (session *Session) BasicLogin(username string, password string) error {
	// Check if the 'basic' method is actually supported by the Reva instance; only continue if this is the case
	supportedMethods, err := session.GetLoginMethods()
	if err != nil {
		return fmt.Errorf("unable to get a list of all supported login methods: %v", err)
	}

	if common.FindStringNoCase(supportedMethods, "basic") == -1 {
		return fmt.Errorf("'basic' login method is not supported")
	}

	return session.Login("basic", username, password)
}

// NewHTTPRequest returns an HTTP request instance.
func (session *Session) NewHTTPRequest(endpoint string, method string, transportToken string, data io.Reader) (*net.HTTPRequest, error) {
	return net.NewHTTPRequest(session.ctx, endpoint, method, session.token, transportToken, data)
}

// Client gets the gateway client instance.
func (session *Session) Client() gateway.GatewayAPIClient {
	return session.client
}

// Context returns the session context.
func (session *Session) Context() context.Context {
	return session.ctx
}

// Token returns the session token.
func (session *Session) Token() string {
	return session.token
}

// IsValid checks whether the session has been initialized and fully established.
func (session *Session) IsValid() bool {
	return session.client != nil && session.ctx != nil && session.token != ""
}

// NewSessionWithContext creates a new Reva session using the provided context.
func NewSessionWithContext(ctx context.Context) (*Session, error) {
	session := &Session{}
	if err := session.initSession(ctx); err != nil {
		return nil, fmt.Errorf("unable to initialize the session: %v", err)
	}
	return session, nil
}

// NewSession creates a new Reva session using a default background context.
func NewSession() (*Session, error) {
	return NewSessionWithContext(context.Background())
}

// MustNewSession creates a new session and panics on failure.
func MustNewSession() *Session {
	session, err := NewSession()
	if err != nil {
		panic(err)
	}
	return session
}
