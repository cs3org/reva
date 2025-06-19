---
title: "emailhandler"
linkTitle: "emailhandler"
weight: 10
description: >
  Configuration for the emailhandler service
---

# _struct: config_

{{% dir name="smtp_server" type="string" default="" %}}
The hostname and port of the SMTP server. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/notification/handler/emailhandler/emailhandler.go#L46)
{{< highlight toml >}}
[notification.handler.emailhandler]
smtp_server = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="sender_login" type="string" default="" %}}
The email to be used to send mails. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/notification/handler/emailhandler/emailhandler.go#L47)
{{< highlight toml >}}
[notification.handler.emailhandler]
sender_login = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="sender_password" type="string" default="" %}}
The sender's password. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/notification/handler/emailhandler/emailhandler.go#L48)
{{< highlight toml >}}
[notification.handler.emailhandler]
sender_password = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="disable_auth" type="bool" default=false %}}
Whether to disable SMTP auth. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/notification/handler/emailhandler/emailhandler.go#L49)
{{< highlight toml >}}
[notification.handler.emailhandler]
disable_auth = false
{{< /highlight >}}
{{% /dir %}}

{{% dir name="default_sender" type="string" default="no-reply@cernbox.cern.ch" %}}
Default sender when not specified in the trigger. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/notification/handler/emailhandler/emailhandler.go#L50)
{{< highlight toml >}}
[notification.handler.emailhandler]
default_sender = "no-reply@cernbox.cern.ch"
{{< /highlight >}}
{{% /dir %}}

