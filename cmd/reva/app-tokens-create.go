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

package main

import (
	"io"
	"strings"

	applicationsv1beta1 "github.com/cs3org/go-cs3apis/cs3/auth/applications/v1beta1"
	v1beta11 "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
)

type AppTokenCreateOpts struct {
	Expiration string
	Label      string
	Path       string
	Share      string
	Unlimited  bool
}

var appTokensCreateOpts *AppTokenCreateOpts = &AppTokenCreateOpts{}

const layoutTime = "2006-01-02T15:04"

func appTokensCreateCommand() *command {
	cmd := newCommand("token-create")
	cmd.Description = func() string { return "create a new application tokens" }
	cmd.Usage = func() string { return "Usage: token-create" }

	cmd.StringVar(&appTokensCreateOpts.Label, "label", "", "set a label")
	cmd.StringVar(&appTokensCreateOpts.Expiration, "expiration", "", "set expiration time (format <yyyy-mm-dd hh:mm>)")
	cmd.StringVar(&appTokensCreateOpts.Path, "path", "", "TODO")
	cmd.StringVar(&appTokensCreateOpts.Share, "share", "", "TODO")
	cmd.BoolVar(&appTokensCreateOpts.Unlimited, "all", false, "TODO")

	cmd.ResetFlags = func() {
		// TODO: reset flags
	}

	cmd.Action = func(w ...io.Writer) error {

		client, err := getClient()
		if err != nil {
			return err
		}

		ctx := getAuthContext()

		scope, err := getScope(appTokensCreateOpts)
		if err != nil {
			return err
		}

		client.GenerateAppPassword(ctx, &applicationsv1beta1.GenerateAppPasswordRequest{
			Expiration: nil, // TODO: add expiration time
			Label:      appTokensCreateOpts.Label,
			TokenScope: scope,
		})

		return nil
	}

	return cmd
}

func getScope(opts *AppTokenCreateOpts) (map[string]*v1beta11.Scope, error) {
	switch {
	case opts.Path != "":
		// TODO: verify path format
		// path = /path/a/b:rw
		pathPerm := strings.Split(opts.Path, ":")
		path, perm := pathPerm[0], pathPerm[1]
		
	}

	return nil, nil
}
