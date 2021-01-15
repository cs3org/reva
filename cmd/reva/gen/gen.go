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

package gen

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"text/template"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
)

var baseTemplate = `# This config file will start a reva instance that:
# - stores files in the local storage
# - reads users from users.json
# - uses basic authentication to authenticate requests

# basic setup

[core]
max_cpus = "2"

[log]
output = "stdout"
mode = "dev"
level = "debug"

# What services, http middlewares and grpc interceptors should be started?

[http]
enabled_services = ["datasvc", "ocdav", "ocssvc"{{if eq .CredentialStrategy "oidc"}}, "oidcprovider", "wellknown"{{end}}]
enabled_middlewares = ["log", "trace", "auth"{{if eq .CredentialStrategy "oidc"}}, "cors"{{end}}]
network = "tcp"
address = "0.0.0.0:9998"

[grpc]
enabled_services = ["authsvc", "usershareprovidersvc", "storageregistrysvc", "storageprovidersvc"]
enabled_interceptors = ["auth", "prometheus", "log", "trace"]
network = "tcp"
address = "0.0.0.0:9999"
access_log = "stderr"

# Order and configuration of http middleware any grpc interceptors

# HTTP middlewares

[http.middlewares.trace]
priority = 100
header = "x-trace"

[http.middlewares.log]
priority = 200

[http.middlewares.auth]
priority = 300
authsvc = "127.0.0.1:9999"
credential_strategy = "{{.CredentialStrategy}}"
token_strategy = "header"
token_writer = "header"
token_manager = "jwt"
{{if eq .CredentialStrategy "oidc"}}
skip_methods = [
    "/status.php",
    "/oauth2",
    "/oauth2/auth",
    "/oauth2/token",
    "/oauth2/introspect",
    "/oauth2/userinfo",
    "/oauth2/sessions",
    "/.well-known/openid-configuration"
]

[http.middlewares.cors]
priority = 400
allowed_origins = ["*"]
allow_credentials = true
allowed_methods = ["OPTIONS", "GET", "PUT", "POST", "DELETE", "MKCOL", "PROPFIND", "PROPPATCH", "MOVE", "COPY", "REPORT", "SEARCH"]
allowed_headers = ["Origin", "Accept", "Content-Type", "X-Requested-With", "Authorization", "Ocs-Apirequest"]
options_passthrough = true
{{else}}
skip_methods = ["/status.php"]
{{end}}

[http.middlewares.auth.token_managers.jwt]
secret = "{{.TokenSecret}}"

# GRPC interceptors

[grpc.interceptors.trace]
priority = 100
header = "x-trace"

[grpc.interceptors.log]
priority = 200

[grpc.interceptors.prometheus]
priority = 300

[grpc.interceptors.auth]
priority = 400
# keys for grpc metadata are always lowercase, so interceptors headers need to use lowercase.
token_strategy = "header"
token_manager = "jwt"
# GenerateAccessToken contains the credentials in the payload. Skip auth, otherwise services cannot obtain a token.
skip_methods = ["/cs3.authproviderv1beta1.AuthService/GenerateAccessToken"]

[grpc.interceptors.auth.token_managers.jwt]
secret = "{{.TokenSecret}}"

# HTTP services

[http.services.ocdav]
prefix = ""
chunk_folder = "/var/tmp/owncloud/chunks"
storageregistrysvc = "127.0.0.1:9999"
storageprovidersvc = "127.0.0.1:9999"

[http.services.ocssvc]
prefix = "ocs"
usershareprovidersvc = "127.0.0.1:9999"
storageprovidersvc = "127.0.0.1:9999"
# the list of share recipients is taken fro the user.json file
user_manager = "json"

[http.services.ocssvc.user_managers.json]
users = "users.json"

[http.services.ocssvc.config]
version = "1.8"
website = "nexus"
host = "https://localhost:9998"
contact = "admin@localhost"
ssl = "true"
[http.services.ocssvc.capabilities.capabilities.core]
poll_interval = 60
webdav_root = "remote.php/webdav"
[http.services.ocssvc.capabilities.capabilities.core.status]
installed = true
maintenance = false
needsDbUpgrade = false
version = "10.0.9.5"
versionstring = "10.0.9"
edition = "community"
productname = "reva"
hostname = ""
[http.services.ocssvc.capabilities.capabilities.checksums]
supported_types = ["SHA256"]
preferred_upload_type = "SHA256"
[http.services.ocssvc.capabilities.capabilities.files]
private_links = true
bigfilechunking = true
blacklisted_files = ["foo"]
undelete = true
versioning = true
[http.services.ocssvc.capabilities.capabilities.files.tus_support]
version = "1.0.0"
resumable = "1.0.0"
extension = "creation,creation-with-upload"
http_method_override = ""
max_chunk_size = 0
[http.services.ocssvc.capabilities.capabilities.dav]
chunking = "" # set to "1.0" for experimental support
[http.services.ocssvc.capabilities.capabilities.files_sharing]
api_enabled = true
resharing = true
group_sharing = true
auto_accept_share = true
share_with_group_members_only = true
share_with_membership_groups_only = true
default_permissions = 22
search_min_length = 3
[http.services.ocssvc.capabilities.capabilities.files_sharing.public]
enabled = true
send_mail = true
social_share = true
upload = true
multiple = true
supports_upload_only = true
[http.services.ocssvc.capabilities.capabilities.files_sharing.public.password]
enforced = true
[http.services.ocssvc.capabilities.capabilities.files_sharing.public.password.enforced_for]
read_only = true
read_write = true
upload_only = true
[http.services.ocssvc.capabilities.capabilities.files_sharing.public.expire_date]
enabled = true
[http.services.ocssvc.capabilities.capabilities.files_sharing.user]
send_mail = true
[http.services.ocssvc.capabilities.capabilities.files_sharing.user_enumeration]
enabled = true
group_members_only = true
[http.services.ocssvc.capabilities.capabilities.files_sharing.federation]
outgoing = true
incoming = true
[http.services.ocssvc.capabilities.capabilities.notifications]
endpoints = ["list", "get", "delete"]
[http.services.ocssvc.capabilities.version]
edition = "nexus"
major = 10
minor = 0
micro = 11
string = "10.0.11"

[http.services.datasvc]
driver = "{{.DataDriver}}"
prefix = "data"
temp_folder = "/var/tmp/"

{{if eq .DataDriver "local"}}
[http.services.datasvc.drivers.local]
root = "{{.DataPath}}"
{{end}}
{{if eq .DataDriver "owncloud"}}
[http.services.datasvc.drivers.owncloud]
datadirectory = "{{.DataPath}}"
{{end}}

{{if eq .CredentialStrategy "oidc"}}
[http.services.wellknown]
prefix = ".well-known"

[http.services.oidcprovider]
prefix = "oauth2"
{{end}}

# GRPC services

## The authentication service

[grpc.services.authsvc]
token_manager = "jwt"
{{if eq .CredentialStrategy "oidc"}}
# users are authorized by inspecting oidc tokens
auth_manager = "oidc"
# user info is read from the oidc userinfo endpoint
user_manager = "oidc"

[grpc.services.authsvc.auth_managers.oidc]
provider = "http://localhost:9998"
insecure = true
# the client credentials for the token introspection backchannel
client_id = "reva"
client_secret = "foobar"
{{else}}
# users are authorized by checking their password matches the one in the users.json file
auth_manager = "json"
# user info is read from the user.json file
user_manager = "json"

[grpc.services.authsvc.auth_managers.json]
users = "users.json"

[grpc.services.authsvc.user_managers.json]
users = "users.json"
{{end}}

[grpc.services.authsvc.token_managers.jwt]
secret = "{{.TokenSecret}}"

## The storage registry service

[grpc.services.storageregistrysvc]
driver = "static"

[grpc.services.storageregistrysvc.drivers.static.rules]
"/" = "127.0.0.1:9999"
"123e4567-e89b-12d3-a456-426655440000" = "127.0.0.1:9999"

## The storage provider service

[grpc.services.storageprovidersvc]
driver = "{{.DataDriver}}"
mount_path = "/"
mount_id = "123e4567-e89b-12d3-a456-426655440000"
data_server_url = "http://127.0.0.1:9998/data"

[grpc.services.storageprovidersvc.available_checksums]
md5   = 100
unset = 1000

{{if eq .DataDriver "local"}}
[grpc.services.storageprovidersvc.drivers.local]
root = "{{.DataPath}}"
{{end}}
{{if eq .DataDriver "owncloud"}}
[grpc.services.storageprovidersvc.drivers.owncloud]
datadirectory = "{{.DataPath}}"
{{end}}

## The user share provider service

[grpc.services.usershareprovidersvc]
driver = "{{.DataDriver}}"

{{if eq .DataDriver "local"}}
[grpc.services.usershareprovidersvc.drivers.local]
root = "{{.DataPath}}"
{{end}}
{{if eq .DataDriver "owncloud"}}
[grpc.services.usershareprovidersvc.drivers.owncloud]
datadirectory = "{{.DataPath}}"
{{end}}
`

// Variables that will be used to render the template
type Variables struct {
	CredentialStrategy string
	TokenSecret        string
	DataDriver         string
	DataPath           string
}

func genSecret(l int) string {
	buff := make([]byte, l)
	_, err := rand.Read(buff)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading random: %v\n", err)
		return ""
	}
	return base64.StdEncoding.EncodeToString(buff)[:l]

}

// WriteConfig writes a basic auth protected reva.toml file to the given path
func WriteConfig(p string, cs string, dd string, dp string) {

	v := &Variables{
		CredentialStrategy: cs,
		TokenSecret:        genSecret(32),
		DataDriver:         dd,
		DataPath:           dp,
	}

	tmpl, err := template.New("config").Parse(baseTemplate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing config template: %v\n", err)
		return
	}
	f, err := os.Create(p)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating config file: %v\n", err)
		return
	}
	if err := tmpl.Execute(f, v); err != nil {
		fmt.Fprintf(os.Stderr, "error writing config file: %v\n", err)
		return
	}
	fmt.Fprintf(os.Stdout, "wrote %s\n", p)
}

var usersTemplate = `[{{range  $i, $e := .}}{{if $i}},{{end}}
	{
		"id": {
			"idp": "{{$e.Iss}}",
			"opaque_id": "{{$e.Sub}}",
		},
		"username": "{{$e.Username}}",
		"secret": "{{$e.Secret}}",
		"mail": "{{$e.Mail}}",
		"display_name": "{{$e.Displayname}}"
	}{{end}}
]
`

// UserVars that will be used to render users
type UserVars struct {
	Sub         string
	Iss         string
	Username    string
	Secret      string
	Mail        string
	Displayname string
	// TODO groups
}

// WriteUsers writes a basic auth protected reva.toml file to the given path
func WriteUsers(p string, users []*userpb.User) {

	var uservars []*UserVars

	if users == nil {
		uservars = []*UserVars{
			&UserVars{
				Sub:         "c6e5995d6c7fa1986b830b78b478e6c2",
				Iss:         "localhost:9998",
				Username:    "einstein",
				Secret:      "relativity",
				Mail:        "einstein@example.org",
				Displayname: "Albert Einstein",
			},
			&UserVars{
				Sub:         "9fb5f8d212cbf3fc55f1bf67d97ed05d",
				Iss:         "localhost:9998",
				Username:    "marie",
				Secret:      "radioactivity",
				Mail:        "marie@example.org",
				Displayname: "Marie Curie",
			},
			&UserVars{
				Sub:         "a84075b398fe6a0aee1155f8ead13331",
				Iss:         "localhost:9998",
				Username:    "richard",
				Secret:      "superfluidity",
				Mail:        "richard@example.org",
				Displayname: "Richard Feynman",
			},
		}
	} else {
		hasher := md5.New()
		uservars = []*UserVars{}
		for _, user := range users {
			// TODO this could be parameterized to create an admin account?
			u := &UserVars{
				Username:    user.Username,
				Secret:      genSecret(12),
				Mail:        user.Mail,
				Displayname: user.DisplayName,
			}
			if user.Id != nil {
				u.Sub = user.Id.OpaqueId
				u.Iss = user.Id.Idp
			}
			// fall back to hashing a username if no sub is provided
			if u.Sub == "" {
				_, err := hasher.Write([]byte(user.Username))
				if err != nil {
					fmt.Fprintf(os.Stderr, "error hashing username: %v\n", err)
					return
				}
				u.Sub = hex.EncodeToString(hasher.Sum(nil))
			}
			uservars = append(uservars, u)
		}
	}

	tmpl, err := template.New("users").Parse(usersTemplate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing config template: %v\n", err)
		return
	}
	f, err := os.Create(p)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating config file: %v\n", err)
		return
	}
	if err := tmpl.Execute(f, uservars); err != nil {
		fmt.Fprintf(os.Stderr, "error writing config file: %v\n", err)
		return
	}
	fmt.Fprintf(os.Stdout, "wrote %s\n", p)
}
