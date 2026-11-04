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

package grpc

import (
	"context"
	"testing"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	mockgateway "github.com/cs3org/go-cs3apis/mocks/github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
	"github.com/stretchr/testify/mock"
)

func space(id, name, typ, path string) *provider.StorageSpace {
	return &provider.StorageSpace{Id: &provider.StorageSpaceId{OpaqueId: id}, Name: name, SpaceType: typ, RootInfo: &provider.ResourceInfo{Path: path}}
}

func TestResolveSpace(t *testing.T) {
	gw := mockgateway.NewMockGatewayAPIClient(t)
	gw.On("ListStorageSpaces", mock.Anything, mock.Anything).Return(&provider.ListStorageSpacesResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
		StorageSpaces: []*provider.StorageSpace{
			space("eoshome-a$alice", "alice", "personal", "/eos/user/a/alice"),
			space("eosproject-c$cernbox", "cernbox", "project", "/eos/project/c/cernbox"),
			space("eosproject-c$cernbox-docs", "cernbox-docs", "project", "/eos/project/c/cernbox/docs"),
		},
	}, nil)
	b := New(func(ctx context.Context) (gateway.GatewayAPIClient, context.Context, error) { return gw, ctx, nil })

	for alias, want := range map[string]string{
		"home":                         "/eos/user/a/alice",
		"cernbox":                      "/eos/project/c/cernbox",
		"eosproject-c$cernbox":         "/eos/project/c/cernbox",
		"cernbox-docs":                 "/eos/project/c/cernbox/docs",
		"/eos/project/c/cernbox/a.txt": "/eos/project/c/cernbox",
		"/eos/project/c/cernbox/docs/": "/eos/project/c/cernbox/docs",
	} {
		ref, err := b.Files().ResolveSpace(context.Background(), alias)
		if err != nil || ref.GetPath() != want {
			t.Errorf("%s: got %v, %v, want %s", alias, ref, err, want)
		}
	}
	if _, err := b.Files().ResolveSpace(context.Background(), "atlas"); !backend.IsNotFound(err) {
		t.Errorf("an unknown space: err = %v, want not found", err)
	}
}
