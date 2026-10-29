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

package auth

import (
	"context"
	"sync"

	"github.com/cs3org/reva/v3/pkg/cli/kerberos"
)

// KerberosProvider turns the Kerberos ticket of kinit into a SPNEGO token.
type KerberosProvider struct {
	Ticket kerberos.Provider
	// SPN returns the service principal of the server, for example
	// HTTP/cernbox.cern.ch.
	SPN func() (string, error)
	// Negotiate builds the SPNEGO token. Nil means Ticket.Negotiate. Tests set
	// it, because a real token needs a KDC.
	Negotiate func(spn string) (string, error)

	once   sync.Once
	ticket *kerberos.Ticket
	tkErr  error
}

// Name implements Provider.
func (p *KerberosProvider) Name() string { return MethodKerberos }

// Available implements Provider.
func (p *KerberosProvider) Available(context.Context) bool {
	_, err := p.load()
	return err == nil
}

// IdentityHint implements IdentityHinter.
func (p *KerberosProvider) IdentityHint() string {
	t, err := p.load()
	if err != nil {
		return ""
	}
	return t.Principal
}

func (p *KerberosProvider) load() (*kerberos.Ticket, error) {
	p.once.Do(func() { p.ticket, p.tkErr = p.Ticket.Ticket() })
	return p.ticket, p.tkErr
}

// Token implements Provider.
func (p *KerberosProvider) Token(context.Context) (*Token, error) {
	t, err := p.load()
	if err != nil {
		return nil, err
	}
	spn, err := p.SPN()
	if err != nil {
		return nil, err
	}
	negotiate := p.Negotiate
	if negotiate == nil {
		negotiate = p.Ticket.Negotiate
	}
	v, err := negotiate(spn)
	if err != nil {
		return nil, err
	}
	return &Token{Kind: KindKerberos, Value: v, User: t.Username(), Provider: MethodKerberos, Subject: t.Principal}, nil
}
