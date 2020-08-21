---
title: "smtpclient"
linkTitle: "smtpclient"
weight: 10
description: >
  Configuration for the smtpclient service
---

# _struct: SMTPCredentials_

{{% dir name="sender_login" type="string" default="" %}}
The login to be used by sender. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/smtpclient/smtpclient.go#L36)
{{< highlight toml >}}
[smtpclient]
sender_login = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="sender_mail" type="string" default="" %}}
The email to be used to send mails. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/smtpclient/smtpclient.go#L37)
{{< highlight toml >}}
[smtpclient]
sender_mail = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="sender_password" type="string" default="" %}}
The sender's password. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/smtpclient/smtpclient.go#L38)
{{< highlight toml >}}
[smtpclient]
sender_password = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="smtp_server" type="string" default="" %}}
The hostname of the SMTP server. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/smtpclient/smtpclient.go#L39)
{{< highlight toml >}}
[smtpclient]
smtp_server = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="smtp_port" type="int" default=587 %}}
The port on which the SMTP daemon is running. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/smtpclient/smtpclient.go#L40)
{{< highlight toml >}}
[smtpclient]
smtp_port = 587
{{< /highlight >}}
{{% /dir %}}

{{% dir name="disable_auth" type="bool" default=false %}}
Whether to disable SMTP auth. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/smtpclient/smtpclient.go#L41)
{{< highlight toml >}}
[smtpclient]
disable_auth = false
{{< /highlight >}}
{{% /dir %}}

{{% dir name="local_name" type="string" default="" %}}
The host name to be used for unauthenticated SMTP. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/smtpclient/smtpclient.go#L42)
{{< highlight toml >}}
[smtpclient]
local_name = ""
{{< /highlight >}}
{{% /dir %}}

