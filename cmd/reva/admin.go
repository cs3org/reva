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

package main

import (
	"io"

	"github.com/cs3org/reva/v3/cmd/reva/admin"
)

// adminCommand is the top-level `admin` entry. The admin CLI itself lives in its
// own package (cmd/reva/admin); this shim just hands it the transport flags and
// access to the persisted admin/login state.
func adminCommand() *command {
	cmd := newCommand("admin")
	cmd.Description = func() string { return "administer the reva fleet (run `admin` for subcommands)" }
	cmd.Action = func(w ...io.Writer) error {
		return admin.Dispatch(cmd.Args(), admin.Options{
			Insecure:   insecure,
			SkipVerify: skipverify,
			AdminHost: func() string {
				if conf != nil {
					return conf.AdminHost
				}
				return ""
			},
			PersistAdminHost: persistAdminHost,
			LoginToken:       readToken,
		})
	}
	return cmd
}

// persistAdminHost stores the admin host in the shared reva config, preserving
// the gateway host.
func persistAdminHost(hostVal string) {
	c, err := readConfig()
	if err != nil || c == nil {
		c = &config{}
		if conf != nil {
			c.Host = conf.Host
		}
	}
	c.AdminHost = hostVal
	_ = writeConfig(c)
	if conf != nil {
		conf.AdminHost = hostVal
	} else {
		conf = c
	}
}
