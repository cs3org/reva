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

package revacmd

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/cs3org/reva/v3/internal/cli/state"
	"github.com/cs3org/reva/v3/pkg/cli"
	"github.com/cs3org/reva/v3/pkg/cli/kerberos"
	"github.com/spf13/cobra"
)

// kerberosLogin asks for no password. The server takes the user from the ticket.
func kerberosLogin(cmd *cobra.Command, rt *cli.Runtime, spn, ccache string) error {
	if rt.AppTokenFile() != "" {
		return errors.New("--app-token-file does not apply to --method kerberos")
	}
	p := &kerberos.Provider{CCache: ccache}
	ticket, err := p.Ticket()
	if err != nil {
		return err
	}
	if rt.User() != "" && rt.User() != ticket.Username() {
		return fmt.Errorf("the Kerberos ticket in %s is for %s, not %s: run `kinit %s`, or leave out --user",
			ticket.CachePath, ticket.Principal, rt.User(), rt.User())
	}
	if !cmd.Flags().Changed("spn") {
		if spn, err = kerberosSPN(rt); err != nil {
			return err
		}
	}
	rt.Debugf("kerberos: principal %s, cache %s, service principal %s", ticket.Principal, ticket.CachePath, spn)
	secret, err := p.Negotiate(spn)
	if err != nil {
		return err
	}
	token, err := rt.Authenticate(cmd.Context(), "kerberos", ticket.Username(), secret)
	if err != nil {
		return err
	}
	if err := state.SetToken(token); err != nil {
		return err
	}
	rt.Msg(cmd.ErrOrStderr(), "Signed in as %s using kerberos (%s).", ticket.Username(), ticket.Principal)
	return nil
}

func kerberosSPN(rt *cli.Runtime) (string, error) {
	if v := os.Getenv("REVA_KERBEROS_SPN"); v != "" {
		return v, nil
	}
	c, err := state.ReadConfig()
	if err != nil {
		return "", err
	}
	if c.Auth.Kerberos.SPN != "" {
		return c.Auth.Kerberos.SPN, nil
	}
	addr := rt.GatewayAddr()
	// A gRPC target can name a resolver, as in dns:///host:port.
	if _, rest, ok := strings.Cut(addr, ":///"); ok {
		addr = rest
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return "", errors.New("no service principal for Kerberos: pass --spn HTTP/<host>, or set a gateway")
	}
	return "HTTP/" + host, nil
}
