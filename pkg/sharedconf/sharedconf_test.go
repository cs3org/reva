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

package sharedconf

import (
	"testing"
)

func Test(t *testing.T) {
	conf := map[string]interface{}{
		"jwt_secret": "",
		"gateway":    "",
	}

	err := Decode(conf)
	if err != nil {
		t.Fatal(err)
	}

	got := GetJWTSecret("secret")
	if got != "secret" {
		t.Fatalf("expected %q got %q", "secret", got)
	}

	got = GetJWTSecret("")
	if got != "changemeplease" {
		t.Fatalf("expected %q got %q", "changemeplease", got)
	}

	conf = map[string]interface{}{
		"jwt_secret": "dummy",
	}

	err = Decode(conf)
	if err != nil {
		t.Fatal(err)
	}

	got = GetJWTSecret("secret")
	if got != "secret" {
		t.Fatalf("expected %q got %q", "secret", got)
	}

	got = GetJWTSecret("")
	if got != "dummy" {
		t.Fatalf("expected %q got %q", "dummy", got)
	}
}
