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

// Package publisher contains an example implementation for a publisher
package publisher

import (
	"log"

	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/pkg/events"
)

// Example publishes events to the queue.
func Example(p events.Publisher) {
	// nothing to do - just publish!
	err := events.Publish(p, events.ShareCreated{
		Sharer: &user.UserId{
			OpaqueId: "123",
		},
		GranteeUserID: &user.UserId{
			OpaqueId: "456",
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
