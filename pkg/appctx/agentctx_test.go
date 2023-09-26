// Copyright 2018-2023 CERN
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

package appctx

import (
	"context"
	"testing"

	"google.golang.org/grpc/metadata"
)

func TestUserAgentIsAllowed(t *testing.T) {
	tests := []struct {
		description string
		userAgent   string
		expected    string
	}{
		{
			description: "grpc-go",
			userAgent:   "grpc-go",
			expected:    "grpc",
		},
		{
			description: "web-firefox",
			userAgent:   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/15.0 Safari/605.1.15",
			expected:    "web",
		},
		{
			description: "desktop-mirall",
			userAgent:   "Mozilla/5.0 (Linux) mirall/2.7.1 (build 2596) (cernboxcmd, centos-3.10.0-1160.36.2.el7.x86_64 ClientArchitecture: x86_64 OsArchitecture: x86_64)",
			expected:    "desktop",
		},
		{
			description: "mobile-android",
			userAgent:   "Mozilla/5.0 (Android) ownCloud-android/2.13.1 cernbox/Android",
			expected:    "mobile",
		},
		{
			description: "mobile-ios",
			userAgent:   "Mozilla/5.0 (iOS) ownCloud-iOS/3.8.0 cernbox/iOS",
			expected:    "mobile",
		},
		{
			description: "mobile-web",
			userAgent:   "Mozilla/5.0 (Android 11; Mobile; rv:86.0) Gecko/86.0 Firefox/86.0",
			expected:    "web",
		},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			ctx = metadata.NewIncomingContext(ctx, metadata.New(map[string]string{UserAgentHeader: tt.userAgent}))
			cat, _ := ContextGetUserAgentCategory(ctx)

			if cat != tt.expected {
				t.Fatalf("result does not match with expected. got=%+v expected=%+v", cat, tt.expected)
			}
		})
	}
}
