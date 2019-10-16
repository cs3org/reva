// Copyright 2018-2019 CERN
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
	"fmt"
	"log"

	gatewayv0alphapb "github.com/cs3org/go-cs3apis/cs3/gateway/v0alpha"
	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	"github.com/cs3org/reva/pkg/token"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const defaultHeader = "x-access-token"

func getAuthContext() context.Context {
	ctx := context.Background()
	// read token from file
	t, err := readToken()
	if err != nil {
		log.Println(err)
		return ctx
	}
	ctx = token.ContextSetToken(ctx, t)
	ctx = metadata.AppendToOutgoingContext(ctx, defaultHeader, t)
	return ctx
}

func getClient() (gatewayv0alphapb.GatewayServiceClient, error) {
	conn, err := getConn()
	if err != nil {
		return nil, err
	}
	return gatewayv0alphapb.NewGatewayServiceClient(conn), nil
}

func getConn() (*grpc.ClientConn, error) {
	return grpc.Dial(conf.Host, grpc.WithInsecure())
}

func formatError(status *rpcpb.Status) error {
	return fmt.Errorf("error: code=%+v msg=%q support_trace=%q", status.Code, status.Message, status.Trace)
}
