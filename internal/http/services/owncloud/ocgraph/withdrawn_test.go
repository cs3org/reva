// Copyright 2018-2024 CERN
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

package ocgraph

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	"google.golang.org/grpc"
)

type refusingOCMGateway struct {
	gateway.GatewayAPIClient
	status *rpc.Status
	gets   int
}

func (g *refusingOCMGateway) UpdateReceivedOCMShare(context.Context, *ocm.UpdateReceivedOCMShareRequest, ...grpc.CallOption) (*ocm.UpdateReceivedOCMShareResponse, error) {
	return &ocm.UpdateReceivedOCMShareResponse{Status: g.status}, nil
}

func (g *refusingOCMGateway) GetReceivedOCMShare(context.Context, *ocm.GetReceivedOCMShareRequest, ...grpc.CallOption) (*ocm.GetReceivedOCMShareResponse, error) {
	g.gets++
	return &ocm.GetReceivedOCMShareResponse{Status: &rpc.Status{Code: rpc.Code_CODE_INTERNAL}}, nil
}

func TestUnhideWithdrawnOCMShareIsConflict(t *testing.T) {
	// Only the withdrawn refusal is owned here; other statuses keep the
	// existing (inherited) code mapping and must not look withdrawn.
	cases := []struct {
		name      string
		status    *rpc.Status
		withdrawn bool
	}{
		{"withdrawn", &rpc.Status{Code: rpc.Code_CODE_FAILED_PRECONDITION, Message: share.WithdrawnShareMessage}, true},
		{"other precondition", &rpc.Status{Code: rpc.Code_CODE_FAILED_PRECONDITION, Message: "transfer already in progress"}, false},
		{"not found", &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gw := &refusingOCMGateway{status: tc.status}
			rec := httptest.NewRecorder()
			(&svc{}).updateReceivedOCMShare(context.Background(), rec, gw, "7", false)
			if gw.gets != 0 || rec.Code < 400 {
				t.Fatalf("a refused update was reported as a share: %d", rec.Code)
			}
			looksWithdrawn := strings.Contains(rec.Body.String(), share.WithdrawnShareMessage)
			if tc.withdrawn && (rec.Code != http.StatusConflict || !looksWithdrawn) {
				t.Fatalf("withdrawn: status %d body %q", rec.Code, rec.Body.String())
			}
			if !tc.withdrawn && looksWithdrawn {
				t.Fatalf("%s reported as withdrawn", tc.name)
			}
		})
	}
}
