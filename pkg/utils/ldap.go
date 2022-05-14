// Copyright 2021 CERN
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

package utils

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"

	"github.com/go-ldap/ldap/v3"
	"github.com/pkg/errors"
)

// LDAPConn holds the basic parameter for setting up an
// LDAP connection.
type LDAPConn struct {
	Hostname     string `mapstructure:"hostname"`
	Port         int    `mapstructure:"port"`
	Insecure     bool   `mapstructure:"insecure" docs:"false;Whether to skip certificate checks when sending requests."`
	CACert       string `mapstructure:"cacert"`
	BindUsername string `mapstructure:"bind_username"`
	BindPassword string `mapstructure:"bind_password"`
}

// GetLDAPConnection initializes an LDAPS connection and allows
// to set TLS options e.g. to add trusted Certificates or disable
// Certificate verification
func GetLDAPConnection(c *LDAPConn) (*ldap.Conn, error) {
	tlsconfig := &tls.Config{InsecureSkipVerify: c.Insecure}

	if !c.Insecure && c.CACert != "" {
		if pemBytes, err := ioutil.ReadFile(c.CACert); err == nil {
			rpool, _ := x509.SystemCertPool()
			rpool.AppendCertsFromPEM(pemBytes)
			tlsconfig.RootCAs = rpool
		} else {
			return nil, errors.Wrapf(err, "Error reading LDAP CA Cert '%s.'", c.CACert)
		}
	}
	l, err := ldap.DialTLS("tcp", fmt.Sprintf("%s:%d", c.Hostname, c.Port), tlsconfig)
	if err != nil {
		return nil, err
	}

	if c.BindUsername != "" && c.BindPassword != "" {
		err = l.Bind(c.BindUsername, c.BindPassword)
		if err != nil {
			l.Close()
			return nil, err
		}
	}
	return l, nil
}
