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

package userprovider

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/user"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

type failingClaimManager struct {
	user.Manager
	err error
}

func (m failingClaimManager) GetUserByClaim(context.Context, string, string, bool) (*userpb.User, error) {
	return nil, m.err
}

func TestUserIDClaimFailureDiagnostics(t *testing.T) {
	const value = "private-opaque-id"
	const details = "private-backend-details"
	for _, tc := range []struct {
		err  error
		code rpc.Code
	}{
		{errtypes.NotFound(details), rpc.Code_CODE_NOT_FOUND},
		{errtypes.Conflict(details), rpc.Code_CODE_ABORTED},
		{errtypes.NotSupported(details), rpc.Code_CODE_UNIMPLEMENTED},
		{fmt.Errorf("%s: %w", details, errors.New(value)), rpc.Code_CODE_INTERNAL},
		{fmt.Errorf("%s: %w", details, grpcstatus.Error(codes.NotFound, value)), rpc.Code_CODE_NOT_FOUND},
		{grpcstatus.Error(codes.Unavailable, details), rpc.Code_CODE_INTERNAL},
	} {
		t.Run(tc.code.String(), func(t *testing.T) {
			var logs bytes.Buffer
			logger := zerolog.New(&logs)
			ctx := appctx.WithLogger(context.Background(), &logger)
			s := &service{usermgr: failingClaimManager{err: tc.err}}
			res, err := s.GetUserByClaim(ctx, &userpb.GetUserByClaimRequest{Claim: "userid", Value: value})
			if err != nil || res.GetStatus().GetCode() != tc.code || res.GetUser() != nil {
				t.Fatalf("userid failure: %v, %v", res, err)
			}
			if got := res.GetStatus().GetMessage(); got != "userid lookup failed" {
				t.Fatalf("userid status message: %q", got)
			}
			if log := logs.String(); !strings.Contains(log, tc.code.String()) ||
				strings.Contains(log, value) || strings.Contains(log, details) || strings.Contains(log, `"error":`) {
				t.Fatalf("userid diagnostic attributes: %s", log)
			}
		})
	}

	var logs bytes.Buffer
	logger := zerolog.New(&logs)
	s := &service{usermgr: failingClaimManager{err: errtypes.NotFound(details)}}
	res, err := s.GetUserByClaim(appctx.WithLogger(context.Background(), &logger), &userpb.GetUserByClaimRequest{Claim: "mail", Value: value})
	if err != nil || res.GetStatus().GetCode() != rpc.Code_CODE_NOT_FOUND ||
		!strings.Contains(res.GetStatus().GetMessage(), value) || !strings.Contains(logs.String(), value) {
		t.Fatalf("ordinary claim diagnostics: %v, %v", res, err)
	}
}
