// Copyright 2018-2022 CERN
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

package owncloudsql

import (
	"testing"

	"github.com/cs3org/reva/pkg/auth/manager/owncloudsql/accounts"
	"github.com/pkg/errors"
)

// new returns a dummy auth manager for testing.
func new(m map[string]interface{}) (*manager, error) {
	mgr := &manager{}
	err := mgr.Configure(m)
	if err != nil {
		err = errors.Wrap(err, "error creating a new auth manager")
		return nil, err
	}

	mgr.db, err = accounts.New("unused", nil, false, false, false)
	if err != nil {
		return nil, err
	}

	return mgr, nil
}

func TestVerify(t *testing.T) {
	tests := map[string]struct {
		password string
		hash     string
		expected bool
	}{
		// Bogus values
		"bogus-1": {"", "asf32äà$$a.|3", false},
		"bogus-2": {"", "", false},

		// Valid SHA1 strings
		"valid-sha1-1": {"password", "5baa61e4c9b93f3f0682250b6cf8331b7ee68fd8", true},
		"valid-sha1-2": {"owncloud.com", "27a4643e43046c3569e33b68c1a4b15d31306d29", true},

		// Invalid SHA1 strings
		"invalid-sha1-1": {"InvalidString", "5baa61e4c9b93f3f0682250b6cf8331b7ee68fd8", false},
		"invalid-sha1-2": {"AnotherInvalidOne", "27a4643e43046c3569e33b68c1a4b15d31306d29", false},

		// Valid legacy password string with password salt "6Wow67q1wZQZpUUeI6G2LsWUu4XKx"
		"valid-legacy-1": {"password", "$2a$08$emCpDEl.V.QwPWt5gPrqrOhdpH6ailBmkj2Hd2vD5U8qIy20HBe7.", true},
		"valid-legacy-2": {"password", "$2a$08$yjaLO4ev70SaOsWZ9gRS3eRSEpHVsmSWTdTms1949mylxJ279hzo2", true},
		"valid-legacy-3": {"password", "$2a$08$.jNRG/oB4r7gHJhAyb.mDupNUAqTnBIW/tWBqFobaYflKXiFeG0A6", true},
		"valid-legacy-4": {"owncloud.com", "$2a$08$YbEsyASX/hXVNMv8hXQo7ezreN17T8Jl6PjecGZvpX.Ayz2aUyaZ2", true},
		"valid-legacy-5": {"owncloud.com", "$2a$11$cHdDA2IkUP28oNGBwlL7jO/U3dpr8/0LIjTZmE8dMPA7OCUQsSTqS", true},
		"valid-legacy-6": {"owncloud.com", "$2a$08$GH.UoIfJ1e.qeZ85KPqzQe6NR8XWRgJXWIUeE1o/j1xndvyTA1x96", true},

		// Invalid legacy passwords
		"invalid-legacy": {"password", "$2a$08$oKAQY5IhnZocP.61MwP7xu7TNeOb7Ostvk3j6UpacvaNMs.xRj7O2", false},

		// Valid passwords "6Wow67q1wZQZpUUeI6G2LsWUu4XKx"
		"valid-1": {"password", "1|$2a$05$ezAE0dkwk57jlfo6z5Pql.gcIK3ReXT15W7ITNxVS0ksfhO/4E4Kq", true},
		"valid-2": {"password", "1|$2a$05$4OQmloFW4yTVez2MEWGIleDO9Z5G9tWBXxn1vddogmKBQq/Mq93pe", true},
		"valid-3": {"password", "1|$2a$11$yj0hlp6qR32G9exGEXktB.yW2rgt2maRBbPgi3EyxcDwKrD14x/WO", true},
		"valid-4": {"owncloud.com", "1|$2a$10$Yiss2WVOqGakxuuqySv5UeOKpF8d8KmNjuAPcBMiRJGizJXjA2bKm", true},
		"valid-5": {"owncloud.com", "1|$2a$10$v9mh8/.mF/Ut9jZ7pRnpkuac3bdFCnc4W/gSumheQUi02Sr.xMjPi", true},
		"valid-6": {"owncloud.com", "1|$2a$05$ST5E.rplNRfDCzRpzq69leRzsTGtY7k88h9Vy2eWj0Ug/iA9w5kGK", true},

		// Invalid passwords
		"invalid-1": {"password", "0|$2a$08$oKAQY5IhnZocP.61MwP7xu7TNeOb7Ostvk3j6UpacvaNMs.xRj7O2", false},
		"invalid-2": {"password", "1|$2a$08$oKAQY5IhnZocP.61MwP7xu7TNeOb7Ostvk3j6UpacvaNMs.xRj7O2", false},
		"invalid-3": {"password", "2|$2a$08$oKAQY5IhnZocP.61MwP7xu7TNeOb7Ostvk3j6UpacvaNMs.xRj7O2", false},
	}

	u, err := new(map[string]interface{}{
		"legacy_salt": "6Wow67q1wZQZpUUeI6G2LsWUu4XKx",
	})
	if err != nil {
		t.Fatalf("could not initialize owncloudsql auth manager: %v", err)
	}

	for name := range tests {
		var tc = tests[name]
		t.Run(name, func(t *testing.T) {
			actual := u.verify(tc.password, tc.hash)
			if actual != tc.expected {
				t.Fatalf("%v returned wrong verification:\n\tAct: %v\n\tExp: %v", t.Name(), actual, tc.expected)
			}
		})
	}
}
