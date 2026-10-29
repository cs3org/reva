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

// Package kerberos reads the ticket that kinit stored in a FILE: credential
// cache and builds the SPNEGO token that the kerberos auth manager accepts.
// The token goes in the ClientSecret of a gRPC Authenticate call, or after
// "Negotiate " in an HTTP Authorization header.
package kerberos

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"

	krb5client "github.com/jcmturner/gokrb5/v8/client"
	krb5config "github.com/jcmturner/gokrb5/v8/config"
	"github.com/jcmturner/gokrb5/v8/credentials"
	"github.com/jcmturner/gokrb5/v8/iana/nametype"
	"github.com/jcmturner/gokrb5/v8/spnego"
	"github.com/jcmturner/gokrb5/v8/types"
)

// Ticket is the Kerberos credential found in the credential cache.
type Ticket struct {
	// Principal is the client principal with its realm, for example alice@CERN.CH.
	Principal string
	Realm     string
	CachePath string
}

// Username is the principal without its realm.
func (t Ticket) Username() string {
	name, _, _ := strings.Cut(t.Principal, "@")
	return name
}

// Provider builds SPNEGO tokens from a FILE: credential cache.
type Provider struct {
	// CCache is a path or FILE:path. Empty means $KRB5CCNAME, then
	// /tmp/krb5cc_<uid>.
	CCache string
	// Config is the krb5.conf to read. Empty means $KRB5_CONFIG, then
	// /etc/krb5.conf.
	Config string
}

// CCachePath returns the file of the credential cache. It refuses the cache
// types that are not files.
func (p *Provider) CCachePath() (string, error) {
	v, from := p.CCache, "--ccache"
	if v == "" {
		v, from = os.Getenv("KRB5CCNAME"), "KRB5CCNAME"
	}
	if v == "" {
		// The default of MIT Kerberos when krb5.conf sets no default_ccache_name.
		return "/tmp/krb5cc_" + strconv.Itoa(os.Getuid()), nil
	}
	if path, ok := strings.CutPrefix(v, "FILE:"); ok {
		return path, nil
	}
	// A type prefix has at least two letters, so C:\ on Windows is a path.
	if typ, _, ok := strings.Cut(v, ":"); ok && len(typ) > 1 && !strings.ContainsAny(typ, `/\`) {
		return "", fmt.Errorf("the credential cache %s=%s is a %s: cache, and only FILE: caches can be read: "+
			"run `export KRB5CCNAME=FILE:/tmp/krb5cc_%d; kinit` and sign in again", from, v, typ, os.Getuid())
	}
	return v, nil
}

// Ticket reads the credential cache. It refuses a cache that has no valid
// ticket-granting ticket.
func (p *Provider) Ticket() (*Ticket, error) {
	_, t, err := p.load()
	return t, err
}

// Negotiate returns the base64 SPNEGO token for the service principal spn,
// for example HTTP/cernbox.cern.ch. If the cache holds no ticket for spn, it
// asks the KDC of krb5.conf for one.
func (p *Provider) Negotiate(spn string) (string, error) {
	if svc, host, ok := strings.Cut(spn, "/"); !ok || svc == "" || host == "" || strings.Contains(spn, "@") {
		return "", fmt.Errorf("service principal %q: use the form HTTP/host", spn)
	}
	cc, _, err := p.load()
	if err != nil {
		return "", err
	}
	cfg, err := p.krb5Config()
	if err != nil {
		return "", err
	}
	cl, err := krb5client.NewFromCCache(cc, cfg, krb5client.DisablePAFXFAST(true))
	if err != nil {
		return "", fmt.Errorf("reading the tickets of %s: %w", cc.Path, err)
	}
	// gokrb5 does not set GSS_C_MUTUAL_FLAG. reva needs this: the auth
	// provider has no way to send a reply token back to the client.
	s := spnego.SPNEGOClient(cl, spn)
	if err := s.AcquireCred(); err != nil {
		return "", advice(err, spn)
	}
	st, err := s.InitSecContext()
	if err != nil {
		return "", advice(err, spn)
	}
	b, err := st.Marshal()
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

func (p *Provider) load() (cc *credentials.CCache, t *Ticket, err error) {
	path, err := p.CCachePath()
	if err != nil {
		return nil, nil, err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, fmt.Errorf("no Kerberos ticket at %s: run kinit (if krb5.conf keeps the tickets in a "+
			"KEYRING: or KCM: cache, run `KRB5CCNAME=FILE:%s kinit`)", path, path)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("reading the Kerberos credential cache: %w", err)
	}
	damaged := fmt.Errorf("the Kerberos credential cache %s is damaged: run kinit", path)
	if len(b) < 2 {
		return nil, nil, damaged
	}
	// gokrb5 panics on a truncated cache, because it does not check lengths.
	defer func() {
		if recover() != nil {
			cc, t, err = nil, nil, damaged
		}
	}()
	cc = &credentials.CCache{Path: path}
	if err := cc.Unmarshal(b); err != nil {
		return nil, nil, fmt.Errorf("reading the Kerberos credential cache %s: %w", path, err)
	}

	realm := cc.GetClientRealm()
	t = &Ticket{Principal: cc.GetClientPrincipalName().PrincipalNameString() + "@" + realm, Realm: realm, CachePath: path}
	tgt, ok := cc.GetEntry(types.PrincipalName{NameType: nametype.KRB_NT_SRV_INST, NameString: []string{"krbtgt", realm}})
	if !ok {
		return nil, nil, fmt.Errorf("no ticket-granting ticket for %s in %s: run kinit", t.Principal, path)
	}
	// gokrb5 can only renew an expired ticket with the password, so refuse it here
	// with a clear message.
	if !time.Now().Before(tgt.EndTime) {
		return nil, nil, fmt.Errorf("the Kerberos ticket of %s in %s expired at %s: run kinit",
			t.Principal, path, tgt.EndTime.Local().Format(time.RFC3339))
	}
	return cc, t, nil
}

func (p *Provider) krb5Config() (*krb5config.Config, error) {
	path := p.Config
	if path == "" {
		path = os.Getenv("KRB5_CONFIG")
	}
	if path == "" {
		path = "/etc/krb5.conf"
	}
	cfg, err := krb5config.Load(path)
	if err != nil {
		if _, ok := errors.AsType[krb5config.UnsupportedDirective](err); !ok {
			return nil, fmt.Errorf("reading the Kerberos config %s: %w", path, err)
		}
	}
	return cfg, nil
}

// advice adds the probable cause to the errors that gokrb5 reports in KDC
// terms only.
func advice(err error, spn string) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "KDC_ERR_S_PRINCIPAL_UNKNOWN"), strings.Contains(msg, "Server not found in Kerberos database"):
		return fmt.Errorf("the KDC does not know the service principal %s: check --spn. If the host is a DNS "+
			"alias, set 'rdns = false' in [libdefaults] of krb5.conf: %w", spn, err)
	case strings.Contains(msg, "KRB_AP_ERR_TKT_EXPIRED"):
		return fmt.Errorf("the Kerberos ticket has expired: run kinit: %w", err)
	default:
		return fmt.Errorf("building the Kerberos token for %s: %w", spn, err)
	}
}
