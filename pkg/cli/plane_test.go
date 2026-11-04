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

package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func tree() (root, routeAdd, shareList *cobra.Command) {
	root = &cobra.Command{Use: "reva"}
	route := AdminCommand(&cobra.Command{Use: "route", Short: "Manage the storage routes"})
	routeAdd = &cobra.Command{Use: "add", Run: func(*cobra.Command, []string) {}}
	route.AddCommand(routeAdd)
	share := &cobra.Command{Use: "share"}
	shareList = &cobra.Command{Use: "list", Run: func(*cobra.Command, []string) {}}
	shareList.Flags().String("owner", "", "only the shares of this owner")
	shareList.Flags().String("path", "", "only the shares on this path")
	AdminFlag(shareList.Flags(), "owner")
	share.AddCommand(shareList)
	root.AddCommand(route, share)
	return root, routeAdd, shareList
}

func TestPlaneOf(t *testing.T) {
	_, routeAdd, shareList := tree()
	if p, why := PlaneOf(routeAdd); p != AdminPlane || why != "`reva route add`" {
		t.Errorf("route add: %v %q", p, why)
	}
	if p, _ := PlaneOf(shareList); p != UserPlane {
		t.Errorf("share list without an operator option: %v", p)
	}
	_ = shareList.Flags().Set("path", "/home")
	if p, _ := PlaneOf(shareList); p != UserPlane {
		t.Errorf("share list --path: %v", p)
	}
	_ = shareList.Flags().Set("owner", "alice")
	if p, why := PlaneOf(shareList); p != AdminPlane || why != "--owner" {
		t.Errorf("share list --owner: %v %q", p, why)
	}
}

func TestSelectPlane(t *testing.T) {
	for _, tc := range []struct {
		name    string
		rt      Runtime
		admin   bool
		owner   string
		wantErr string
	}{
		{name: "as with a user command", rt: Runtime{as: "bob"}},
		{name: "as with an operator command", rt: Runtime{as: "bob"}, admin: true, wantErr: "`reva route add`"},
		{name: "as with an operator option", rt: Runtime{as: "bob"}, owner: "alice", wantErr: "--owner"},
		{name: "reason not ascii", rt: Runtime{reason: "ticket\n42"}, admin: true, wantErr: "--reason"},
		{name: "reason", rt: Runtime{reason: "INC0042"}, admin: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, routeAdd, shareList := tree()
			cmd := shareList
			if tc.admin {
				cmd = routeAdd
			}
			if tc.owner != "" {
				_ = shareList.Flags().Set("owner", tc.owner)
			}
			err := tc.rt.SelectPlane(cmd)
			if tc.wantErr == "" && err != nil || tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestOperatorHelp(t *testing.T) {
	root, _, shareList := tree()
	OperatorHelp(root)
	var b bytes.Buffer
	shareList.SetOut(&b)
	if err := shareList.Usage(); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	flags, operator, ok := strings.Cut(out, OperatorOptions)
	if !ok {
		t.Fatalf("no operator options group:\n%s", out)
	}
	if !strings.Contains(flags, "--path") || strings.Contains(flags, "--owner") || !strings.Contains(operator, "--owner") {
		t.Errorf("--owner must be only in the operator group:\n%s", out)
	}
	if !strings.Contains(root.Commands()[0].Short, "(admin)") {
		t.Errorf("operator command without (admin): %q", root.Commands()[0].Short)
	}
}
