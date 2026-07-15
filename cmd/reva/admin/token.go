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

package admin

import (
	"os"
	"os/user"
	"path"
)

// getAdminTokenFile is where `admin elevate` stores the short-TTL admin token,
// read by every other admin subcommand (the sudo model).
func getAdminTokenFile() string {
	u, err := user.Current()
	if err != nil {
		panic(err)
	}
	return path.Join(u.HomeDir, ".reva-admin-token")
}

func readAdminToken() (string, error) {
	data, err := os.ReadFile(getAdminTokenFile())
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func writeAdminToken(token string) {
	if err := os.WriteFile(getAdminTokenFile(), []byte(token), 0o600); err != nil {
		panic(err)
	}
}
