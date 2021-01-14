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

package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"

	"google.golang.org/grpc/credentials"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/pkg/token"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func getAuthContext() context.Context {
	ctx := context.Background()
	// read token from file
	t, err := readToken()
	if err != nil {
		log.Println(err)
		return ctx
	}
	ctx = token.ContextSetToken(ctx, t)
	ctx = metadata.AppendToOutgoingContext(ctx, token.TokenHeader, t)
	return ctx
}

func getClient() (gateway.GatewayAPIClient, error) {
	conn, err := getConn()
	if err != nil {
		return nil, err
	}
	return gateway.NewGatewayAPIClient(conn), nil
}

func getConn() (*grpc.ClientConn, error) {
	if insecure {
		return grpc.Dial(conf.Host, grpc.WithInsecure())
	}

	// TODO(labkode): if in the future we want client-side certificate validation,
	// we need to load the client cert here
	tlsconf := &tls.Config{InsecureSkipVerify: skipverify}
	creds := credentials.NewTLS(tlsconf)
	return grpc.Dial(conf.Host, grpc.WithTransportCredentials(creds))
}

func formatError(status *rpc.Status) error {
	return fmt.Errorf("error: code=%+v msg=%q support_trace=%q", status.Code, status.Message, status.Trace)
}
