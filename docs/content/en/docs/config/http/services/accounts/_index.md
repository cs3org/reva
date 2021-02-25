---
title: "accounts"
linkTitle: "accounts"
weight: 10
description: >
    Configuration for the Accounts service
---

{{% pageinfo %}}
The accounts service is used to store and manage user accounts.
{{% /pageinfo %}}

## General settings
{{% dir name="prefix" type="string" default="accounts" %}}
The relative root path of all exposed HTTP endpoints of the service.
{{< highlight toml >}}
[http.services.accounts]
prefix = "/accounts"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="notifications_mail" type="string" default="" %}}
An email address where all notifications are sent to.
{{< highlight toml >}}
[http.services.accounts]
notifications_mail = "notify@example.com"
{{< /highlight >}}
{{% /dir %}}

## SMTP settings
{{% dir name="sender_mail" type="string" default="" %}}
An email address from which all emails are sent.
{{< highlight toml >}}
[http.services.accounts.smtp]
sender_mail = "notify@example.com"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="sender_login" type="string" default="" %}}
The login name.
{{< highlight toml >}}
[http.services.accounts.smtp]
sender_login = "hans"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="sender_password" type="string" default="" %}}
The password for the login.
{{< highlight toml >}}
[http.services.accounts.smtp]
password = "secret"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="smtp_server" type="string" default="" %}}
The SMTP server to use.
{{< highlight toml >}}
[http.services.accounts.smtp]
smtp_server = "smtp.example.com"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="smtp_port" type="int" default="25" %}}
The SMTP server port to use.
{{< highlight toml >}}
[http.services.accounts.smtp]
smtp_port = 25
{{< /highlight >}}
{{% /dir %}}

{{% dir name="disable_auth" type="bool" default="false" %}}
Whether to disable authentication.
{{< highlight toml >}}
[http.services.accounts.smtp]
disable_auth = true
{{< /highlight >}}
{{% /dir %}}

## Storage settings
{{% dir name="driver" type="string" default="file" %}}
The storage driver to use; currently, only `file` is supported.
{{< highlight toml >}}
[http.services.accounts.storage]
driver = "file"
{{< /highlight >}}
{{% /dir %}}

### Storage settings - File driver
{{% dir name="file" type="string" default="" %}}
The file location.
{{< highlight toml >}}
[http.services.accounts.storage.file]
file = "/var/reva/accounts.json"
{{< /highlight >}}
{{% /dir %}}
