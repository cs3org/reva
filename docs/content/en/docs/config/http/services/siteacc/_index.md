---
title: "siteacc"
linkTitle: "siteacc"
weight: 10
description: >
    Configuration for the Site Accounts service
---

{{% pageinfo %}}
The site accounts service is used to store and manage site accounts.
{{% /pageinfo %}}

## General settings
{{% dir name="prefix" type="string" default="accounts" %}}
The relative root path of all exposed HTTP endpoints of the service.
{{< highlight toml >}}
[http.services.siteacc]
prefix = "/siteacc"
{{< /highlight >}}
{{% /dir %}}

## Security settings
{{% dir name="creds_passphrase" type="string" default="" %}}
The passphrase to use when encoding stored credentials. Should be exactly 32 characters long.
{{< highlight toml >}}
[http.services.siteacc.security]
creds_passphrase = "supersecretpasswordthatyouknow!"
{{< /highlight >}}
{{% /dir %}}

## GOCDB settings
{{% dir name="url" type="string" default="" %}}
The external URL of the central GOCDB instance.
{{< highlight toml >}}
[http.services.siteacc.gocdb]
url = "https://www.sciencemesh.eu/gocdb/"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="write_url" type="string" default="" %}}
The external URL of the GOCDB Write API.
{{< highlight toml >}}
[http.services.siteacc.gocdb]
write_url = "https://www.sciencemesh.eu/gocdbpi/"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="apikey" type="string" default="" %}}
The API key for the GOCDB.
{{< highlight toml >}}
[http.services.siteacc.gocdb]
apikey = "verysecret"
{{< /highlight >}}
{{% /dir %}}

## Email settings
{{% dir name="notifications_mail" type="string" default="" %}}
An email address where all notifications are sent to.
{{< highlight toml >}}
[http.services.siteacc.email]
notifications_mail = "notify@example.com"
{{< /highlight >}}
{{% /dir %}}

### SMTP settings
{{% dir name="sender_mail" type="string" default="" %}}
An email address from which all emails are sent.
{{< highlight toml >}}
[http.services.siteacc.email.smtp]
sender_mail = "notify@example.com"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="sender_login" type="string" default="" %}}
The login name.
{{< highlight toml >}}
[http.services.siteacc.email.smtp]
sender_login = "hans"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="sender_password" type="string" default="" %}}
The password for the login.
{{< highlight toml >}}
[http.services.siteacc.email.smtp]
password = "secret"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="smtp_server" type="string" default="" %}}
The SMTP server to use.
{{< highlight toml >}}
[http.services.siteacc.email.smtp]
smtp_server = "smtp.example.com"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="smtp_port" type="int" default="25" %}}
The SMTP server port to use.
{{< highlight toml >}}
[http.services.siteacc.email.smtp]
smtp_port = 25
{{< /highlight >}}
{{% /dir %}}

{{% dir name="disable_auth" type="bool" default="false" %}}
Whether to disable authentication.
{{< highlight toml >}}
[http.services.siteacc.email.smtp]
disable_auth = true
{{< /highlight >}}
{{% /dir %}}

## Storage settings
{{% dir name="driver" type="string" default="file" %}}
The storage driver to use; currently, only `file` is supported.
{{< highlight toml >}}
[http.services.siteacc.storage]
driver = "file"
{{< /highlight >}}
{{% /dir %}}

### Storage settings - File drivers
{{% dir name="sites_file" type="string" default="" %}}
The operators file location.
{{< highlight toml >}}
[http.services.siteacc.storage.file]
operators_file = "/var/reva/operators.json"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="accounts_file" type="string" default="" %}}
The accounts file location.
{{< highlight toml >}}
[http.services.siteacc.storage.file]
accounts_file = "/var/reva/accounts.json"
{{< /highlight >}}
{{% /dir %}}

## Mentix settings
{{% dir name="url" type="string" default="" %}}
The main Mentix URL.
{{< highlight toml >}}
[http.services.siteacc.mentix]
url = "https://iop.example.com/mentix"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="data_endpoint" type="string" default="/sites" %}}
The main data endpoint of Mentix.
{{< highlight toml >}}
[http.services.siteacc.mentix]
data_endpoint = "/data"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="sitereg_endpoint" type="string" default="/sitereg" %}}
The site registration endpoint of Mentix.
{{< highlight toml >}}
[http.services.siteacc.mentix]
sitereg_endpoint = "/register"
{{< /highlight >}}
{{% /dir %}}

## Webserver settings
{{% dir name="url" type="string" default="" %}}
The external URL of the site accounts service.
{{< highlight toml >}}
[http.services.siteacc.webserver]
url = "https://www.sciencemesh.eu/accounts/"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="session_timeout" type="int" default="300" %}}
The session timeout in seconds.
{{< highlight toml >}}
[http.services.siteacc.webserver]
session_timeout = 600
{{< /highlight >}}
{{% /dir %}}

{{% dir name="verify_remote_address" type="bool" default="false" %}}
If true, sessions are only valid if they belong to the same IP. This can cause problems behind proxy servers.
{{< highlight toml >}}
[http.services.siteacc.webserver]
verify_remote_address = true
{{< /highlight >}}
{{% /dir %}}

{{% dir name="log_sessions" type="bool" default="false" %}}
If enabled, debug information about sessions will be printed.
{{< highlight toml >}}
[http.services.siteacc.webserver]
log_sessions = true
{{< /highlight >}}
{{% /dir %}}
