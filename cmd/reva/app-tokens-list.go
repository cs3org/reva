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
	"encoding/gob"
	"io"
	"os"
	"reflect"
	"time"

	applicationsv1beta1 "github.com/cs3org/go-cs3apis/cs3/auth/applications/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/jedib0t/go-pretty/table"
)

type AppTokenListOpts struct {
	Long              bool
	All               bool
	OnlyExpired       bool
	ApplicationFilter string
}

var appTokenListOpts *AppTokenListOpts = &AppTokenListOpts{}

func appTokensListCommand() *command {
	cmd := newCommand("token-list")
	cmd.Description = func() string { return "list all the application tokens" }
	cmd.Usage = func() string { return "Usage: token-list [-flags]" }

	cmd.BoolVar(&appTokenListOpts.Long, "l", false, "long listing")
	cmd.BoolVar(&appTokenListOpts.All, "a", false, "print all tokens, also the expired")
	cmd.BoolVar(&appTokenListOpts.OnlyExpired, "e", false, "print only expired token")
	cmd.StringVar(&appTokenListOpts.ApplicationFilter, "n", "", "filter by application name")

	shortHeader := table.Row{"Password", "Label", "Scope", "Expiration"}
	longHeader := table.Row{"Password", "Scope", "Label", "Expiration", "Creation Time", "Last Used Time"}

	cmd.ResetFlags = func() {
		s := reflect.ValueOf(appTokenListOpts).Elem()
		s.Set(reflect.Zero(s.Type()))
	}

	cmd.Action = func(w ...io.Writer) error {

		client, err := getClient()
		if err != nil {
			return err
		}

		ctx := getAuthContext()
		listResponse, err := client.ListAppPasswords(ctx, &applicationsv1beta1.ListAppPasswordsRequest{})

		if err != nil {
			return err
		}

		if listResponse.Status.Code != rpc.Code_CODE_OK {
			return formatError(listResponse.Status)
		}

		listPw := filter(listResponse.AppPasswords, appTokenListOpts)

		if len(w) == 0 {

			t := table.NewWriter()
			t.SetOutputMirror(os.Stdout)

			if appTokenListOpts.Long {
				t.AppendHeader(longHeader)
			} else {
				t.AppendHeader(shortHeader)
			}

			for _, pw := range listPw {
				if appTokenListOpts.Long {
					t.AppendRow(table.Row{pw.Password, pw.TokenScope, pw.Label, pw.Expiration, pw.Ctime, pw.Utime})
				} else {
					t.AppendRow(table.Row{pw.Password, pw.Label, pw.TokenScope, pw.Expiration})
				}
			}

			t.Render()

		} else {
			enc := gob.NewEncoder(w[0])
			if err := enc.Encode(listPw); err != nil {
				return err
			}
		}
		return nil
	}
	return cmd
}

// Filter the list of app password, based on the option selected by the user
func filter(listPw []*applicationsv1beta1.AppPassword, opts *AppTokenListOpts) (filtered []*applicationsv1beta1.AppPassword) {
	var filters Filters

	//TODO: add label filter
	if opts.OnlyExpired {
		filters = append(filters, &FilterByExpired{})
	} else {
		filters = append(filters, &FilterByNotExpired{})
	}
	if opts.ApplicationFilter != "" {
		filters = append(filters, &FilterByApplicationName{name: opts.ApplicationFilter})
	}
	if opts.All {
		// discard all the filters
		filters = []Filter{&FilterByNone{}}
	}

	for _, pw := range listPw {
		if filters.In(pw) {
			filtered = append(filtered, pw)
		}
	}
	return
}

type Filter interface {
	In(*applicationsv1beta1.AppPassword) bool
}

type FilterByApplicationName struct {
	name string
}
type FilterByNone struct{}
type FilterByExpired struct{}
type FilterByNotExpired struct{}
type Filters []Filter

func (f *FilterByApplicationName) In(pw *applicationsv1beta1.AppPassword) bool {
	for app := range pw.TokenScope {
		if app == f.name {
			return true
		}
	}
	return false
}

func (f *FilterByNone) In(pw *applicationsv1beta1.AppPassword) bool {
	return true
}

func (f *FilterByExpired) In(pw *applicationsv1beta1.AppPassword) bool {
	return pw.Expiration != nil && pw.Expiration.Seconds <= uint64(time.Now().Unix())
}

func (f *FilterByNotExpired) In(pw *applicationsv1beta1.AppPassword) bool {
	return !(&FilterByExpired{}).In(pw)
}

func (f Filters) In(pw *applicationsv1beta1.AppPassword) bool {
	for _, filter := range f {
		if !filter.In(pw) {
			return false
		}
	}
	return true
}
