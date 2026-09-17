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

package kerberos

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/jcmturner/gokrb5/v8/credentials"
	"github.com/jcmturner/gokrb5/v8/gssapi"
	"github.com/jcmturner/gokrb5/v8/keytab"
	"github.com/jcmturner/gokrb5/v8/service"
	"github.com/jcmturner/gokrb5/v8/spnego"
	"github.com/pkg/errors"
)

// ctxCredentials is the context key gokrb5 stores the verified identity under.
// It is unexported there, so the value is repeated here; a mismatch shows up
// immediately as "the ticket verified but carried no identity", which the
// integration test covers.
const ctxCredentials = "github.com/jcmturner/gokrb5/v8/ctxCredentials"

// keytabValidator verifies SPNEGO tokens against a keytab on disk.
type keytabValidator struct {
	conf *config

	// mu guards the lazily loaded keytab. It is reloaded when the file changes
	// on disk, so that a keytab rotation does not need a restart — and a
	// deployment that rotates keytabs on a schedule would otherwise start
	// rejecting every ticket at an arbitrary hour.
	mu      sync.Mutex
	kt      *keytab.Keytab
	modTime int64
	size    int64
}

func newKeytabValidator(c *config) (*keytabValidator, error) {
	v := &keytabValidator{conf: c}
	// Load once now so a missing or unreadable keytab is a startup failure
	// rather than a puzzle at the first login.
	if _, err := v.keytab(); err != nil {
		return nil, err
	}
	return v, nil
}

// keytab returns the parsed keytab, reloading it when the file has changed.
func (v *keytabValidator) keytab() (*keytab.Keytab, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	info, err := os.Stat(v.conf.Keytab)
	if err != nil {
		return nil, errors.Wrapf(err, "kerberos: cannot read the keytab %s", v.conf.Keytab)
	}
	if v.kt != nil && info.ModTime().UnixNano() == v.modTime && info.Size() == v.size {
		return v.kt, nil
	}

	kt, err := keytab.Load(v.conf.Keytab)
	if err != nil {
		return nil, errors.Wrapf(err, "kerberos: cannot parse the keytab %s", v.conf.Keytab)
	}
	v.kt, v.modTime, v.size = kt, info.ModTime().UnixNano(), info.Size()
	return kt, nil
}

// Validate verifies a SPNEGO token against the keytab.
func (v *keytabValidator) Validate(ctx context.Context, token []byte) (*Ticket, error) {
	kt, err := v.keytab()
	if err != nil {
		return nil, err
	}

	settings := []func(*service.Settings){
		service.MaxClockSkew(v.conf.MaxClockSkew()),
		// The PAC is a Microsoft extension carrying group membership. reva
		// resolves groups through its own user provider, so decoding it would
		// be work whose result is discarded — and it makes validation fail
		// against KDCs that do not issue one.
		service.DecodePAC(false),
	}
	if v.conf.ServicePrincipal != "" {
		settings = append(settings, service.KeytabPrincipal(v.conf.ServicePrincipal))
	}

	var spnegoToken spnego.SPNEGOToken
	if err := spnegoToken.Unmarshal(token); err != nil {
		return nil, errors.Wrap(err, "kerberos: token is not a valid SPNEGO token")
	}

	authenticated, authCtx, status := spnego.SPNEGOService(kt, settings...).AcceptSecContext(&spnegoToken)
	if !authenticated {
		return nil, fmt.Errorf("kerberos: %s", describeStatus(status.Code, status.Message))
	}

	// gokrb5 stores the verified identity under a key of its own, not under
	// goidentity.CTXKey — that one is only set on the HTTP path it also
	// provides, which this does not go through.
	id, ok := authCtx.Value(ctxCredentials).(*credentials.Credentials)
	if !ok || id == nil {
		return nil, errors.New("kerberos: the ticket verified but carried no identity")
	}

	ticket := &Ticket{Principal: id.UserName(), Realm: id.Domain()}
	// A principal with an instance ("user/admin") keeps it in the username, and
	// a caller matching against an account name needs only the first component.
	if name, _, found := strings.Cut(ticket.Principal, "/"); found {
		ticket.Principal = name
	}
	return ticket, nil
}

// describeStatus turns a GSS-API status into something an administrator reading
// the log can act on.
//
// The status codes are bit flags rather than an enumeration, so this matches on
// the ones worth distinguishing and falls back to the mechanism's own message.
func describeStatus(code int, message string) string {
	var reason string
	switch code {
	case gssapi.StatusBadMech:
		reason = "the client offered a mechanism that is not Kerberos"
	case gssapi.StatusBadName, gssapi.StatusBadNameType:
		// By far the most likely misconfiguration: the endpoint is a DNS alias
		// and the client asked the KDC for a ticket for the node name, which is
		// not in the keytab.
		reason = "the service principal in the ticket does not match this service; " +
			"if the endpoint is a DNS alias, check that its SPN is in the keytab and " +
			"that clients are not canonicalising the host through reverse DNS"
	case gssapi.StatusDefectiveToken:
		reason = "the token is malformed"
	case gssapi.StatusDefectiveCredential:
		reason = "the credential is malformed"
	case gssapi.StatusCredentialsExpired, gssapi.StatusContextExpired:
		reason = "the ticket has expired"
	case gssapi.StatusBadSig, gssapi.StatusBadMIC:
		reason = "the ticket did not verify against the keytab"
	case gssapi.StatusDuplicateToken, gssapi.StatusOldToken:
		reason = "the ticket has already been used"
	case gssapi.StatusUnauthorized:
		reason = "the ticket is not authorized for this service"
	default:
		reason = "the ticket did not verify"
	}

	if message == "" {
		return fmt.Sprintf("%s (GSS status %d)", reason, code)
	}
	return reason + ": " + message
}
