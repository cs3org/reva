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

package ctx

import (
	"context"

	ua "github.com/mileusna/useragent"
	"google.golang.org/grpc/metadata"
)

// UserAgentHeader is the header used for the user agent
const UserAgentHeader = "x-user-agent"

// ContextGetUserAgent returns the user agent if set in the given context.
// see https://github.com/grpc/grpc-go/issues/1100
func ContextGetUserAgent(ctx context.Context) (*ua.UserAgent, bool) {
	if userAgentStr, ok := ContextGetUserAgentString(ctx); ok {
		userAgent := ua.Parse(userAgentStr)
		return &userAgent, true
	}
	return nil, false
}

func ContextGetUserAgentString(ctx context.Context) (string, bool) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", false
	}
	userAgentLst, ok := md[UserAgentHeader]
	if !ok {
		userAgentLst, ok = md["user-agent"]
		if !ok {
			return "", false
		}
	}
	if len(userAgentLst) == 0 {
		return "", false
	}
	return userAgentLst[0], true
}
