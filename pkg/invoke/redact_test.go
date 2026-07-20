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

package invoke

import "testing"

func TestRedact(t *testing.T) {
	in := map[string]any{
		"address":    "0.0.0.0:9999",
		"jwt_secret": "supersecret",
		"token":      "abc",
		"api_key":    "k",
		"driver":     "sql",
		"drivers": map[string]any{
			"sql": map[string]any{
				"db_password": "hunter2",
				"host":        "localhost",
				"dsn":         "user:pass@tcp(localhost:3306)/reva",
			},
		},
		"peers": []any{
			map[string]any{"url": "postgres://u:p@host/db", "name": "a"},
		},
	}
	out := Redact(in)

	if out["jwt_secret"] != RedactedValue {
		t.Errorf("jwt_secret not redacted: %v", out["jwt_secret"])
	}
	if out["token"] != RedactedValue {
		t.Errorf("token not redacted: %v", out["token"])
	}
	if out["api_key"] != RedactedValue {
		t.Errorf("api_key not redacted: %v", out["api_key"])
	}
	if out["address"] != "0.0.0.0:9999" {
		t.Errorf("address wrongly redacted: %v", out["address"])
	}
	if out["driver"] != "sql" {
		t.Errorf("driver wrongly redacted: %v", out["driver"])
	}

	nested := out["drivers"].(map[string]any)["sql"].(map[string]any)
	if nested["db_password"] != RedactedValue {
		t.Errorf("nested db_password not redacted: %v", nested["db_password"])
	}
	if nested["dsn"] != RedactedValue {
		t.Errorf("nested dsn not redacted: %v", nested["dsn"])
	}
	if nested["host"] != "localhost" {
		t.Errorf("nested host wrongly redacted: %v", nested["host"])
	}

	peer := out["peers"].([]any)[0].(map[string]any)
	if peer["url"] != RedactedValue {
		t.Errorf("DSN-with-credentials value not redacted: %v", peer["url"])
	}
	if peer["name"] != "a" {
		t.Errorf("peer name wrongly redacted: %v", peer["name"])
	}

	// The input must be left untouched.
	if in["jwt_secret"] != "supersecret" {
		t.Errorf("Redact mutated its input")
	}
}
