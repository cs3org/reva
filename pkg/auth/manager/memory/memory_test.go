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

package memory

import (
	"context"
	"testing"
)

var ctx = context.Background()

func TestUserManager(t *testing.T) {
	// get manager
	manager, _ := New(map[string]interface{}{
		"users": map[string]interface{}{
			"admin": map[string]interface{}{
				"id": map[string]interface{}{
					"opaqueId": "ddc2004c-0977-11eb-9d3f-a793888cd0f8",
					"idp":      "http://localhost:9998",
					"type":     1, // user.UserType_USER_TYPE_PRIMARY
				},
				"username":     "admin",
				"secret":       "admin",
				"mail":         "admin@example.org",
				"display_name": "Administrator",
			},
		},
	})

	// Authenticate - positive test
	_, _, err := manager.Authenticate(ctx, "admin", "admin")
	if err != nil {
		t.Fatalf("error while authenticate with correct credentials")
	}

	// Authenticate - negative test
	_, _, err = manager.Authenticate(ctx, "admin", "NotARealPassword")
	if err == nil {
		t.Fatalf("no error (but we expected one) while authenticate with bad credentials")
	}

}
