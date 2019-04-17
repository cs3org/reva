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

package demo

import (
	"context"
	"testing"

	"github.com/cernbox/reva/pkg/token"
)

var ctx = context.Background()

func TestEncodeDecode(t *testing.T) {
	m, _ := New(nil)
	groups := []string{"radium-lovers"}
	claims := token.Claims{
		"username":     "marie",
		"groups":       groups,
		"display_name": "Marie Curie",
		"mail":         "marie@example.org",
	}

	encoded, err := m.MintToken(ctx, claims)
	if err != nil {
		t.Fatal(err)
	}

	decodedClaims, err := m.DismantleToken(ctx, encoded)
	if err != nil {
		t.Fatal(err)
	}

	if claims["username"] != decodedClaims["username"] {
		t.Fatalf("username claims differ: expected=%s got=%s", claims["username"], decodedClaims["username"])
	}
	if claims["display_name"] != decodedClaims["display_name"] {
		t.Fatalf("display_name claims differ: expected=%s got=%s", claims["display_name"], decodedClaims["display_name"])
	}
	if claims["mail"] != decodedClaims["mail"] {
		t.Fatalf("mail claims differ: expected=%s got=%s", claims["mail"], decodedClaims["mail"])
	}

	decodedGroups, ok := decodedClaims["groups"].([]string)
	if !ok {
		t.Fatal("groups key in decoded claims is not []string")
	}

	if len(groups) != len(groups) {
		t.Fatalf("groups claims differ in length: expected=%d got=%d", len(groups), len(decodedGroups))
	}
}
