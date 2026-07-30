---
title: "handlers"
linkTitle: "handlers"
weight: 10
description: >
  Configuration for the handlers service
---

# _struct: EmailConfig_

{{% dir name="smtp_server" type="string" default="" %}}
The hostname and port of the SMTP server. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/notifications/handlers/email.go#L53)
{{< highlight toml >}}
[notifications.handlers]
smtp_server = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="sender_login" type="string" default="" %}}
The email to be used to send mails. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/notifications/handlers/email.go#L54)
{{< highlight toml >}}
[notifications.handlers]
sender_login = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="sender_password" type="string" default="" %}}
The sender's password. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/notifications/handlers/email.go#L55)
{{< highlight toml >}}
[notifications.handlers]
sender_password = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="disable_auth" type="bool" default=false %}}
Whether to disable SMTP auth. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/notifications/handlers/email.go#L56)
{{< highlight toml >}}
[notifications.handlers]
disable_auth = false
{{< /highlight >}}
{{% /dir %}}

{{% dir name="default_sender" type="string" default="no-reply@cernbox.cern.ch" %}}
Default sender when not specified in the request. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/notifications/handlers/email.go#L57)
{{< highlight toml >}}
[notifications.handlers]
default_sender = "no-reply@cernbox.cern.ch"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="cid_folder" type="string" default="/etc/revad/cid/" %}}
Folder containing files to embed as CIDs in emails. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/notifications/handlers/email.go#L58)
{{< highlight toml >}}
[notifications.handlers]
cid_folder = "/etc/revad/cid/"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="templates" type="map[string]any" default=nil %}}
Email notification templates. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/notifications/handlers/email.go#L59)
{{< highlight toml >}}
[notifications.handlers]
templates = nil
{{< /highlight >}}
{{% /dir %}}

