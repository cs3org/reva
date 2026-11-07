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

package ocmd

import (
	"encoding/json"
	"os"
	"testing"
)

func TestContactContractCorpus(t *testing.T) {
	data, err := os.ReadFile("testdata/contact-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Cases []struct {
			ID      string          `json:"id"`
			Valid   bool            `json:"valid"`
			Payload json.RawMessage `json:"payload"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus.Cases) == 0 {
		t.Fatal("empty contact corpus")
	}
	for _, tc := range corpus.Cases {
		t.Run(tc.ID, func(t *testing.T) {
			n, err := NewNotificationRegistry().Parse(tc.Payload)
			if (err == nil) != tc.Valid {
				t.Errorf("valid = %t, want %t: %v", err == nil, tc.Valid, err)
			}
			if tc.ID == "contact-valid" && err == nil {
				if n.Class != NotificationClassSpec || n.User == nil || n.User.UserID != "alice@alice.example.org" || n.User.TargetUserID != "bob@bob.example.org" {
					t.Error("contact actor and target were not retained as a known notification")
				}
			}
		})
	}
}
