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
	"testing"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	mockgateway "github.com/cs3org/go-cs3apis/mocks/github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	"github.com/stretchr/testify/mock"
)

func TestNestedResourceByAbsolutePath(t *testing.T) {
	shareRoot := &provider.ResourceId{StorageId: "eos", OpaqueId: "share"}
	gtw := mockgateway.NewMockGatewayAPIClient(t)
	gtw.On("Stat", mock.Anything, mock.Anything).Return(&provider.StatResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
		Info:   &provider.ResourceInfo{Id: shareRoot, Path: "/eos/project/p/proj/share"},
	}, nil)

	for p, want := range map[string]bool{
		"/eos/project/p/proj/share":                  true,
		"/eos/project/p/proj/share/sub/file":         true,
		"/eos/project/p/proj/share-secret":           false,
		"/eos/project/p/proj/share/../secret":        false,
		"/eos/project/p/proj/share/sub/../../secret": false,
		"/eos/project/p/proj/share/sub/../file":      true,
	} {
		got, err := checkIfNestedResource(context.Background(), &provider.Reference{Path: p}, shareRoot, nil, gtw, nil)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if got != want {
			t.Errorf("%s: nested = %v, want %v", p, got, want)
		}
	}
}
