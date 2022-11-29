---
title: "mailer"
linkTitle: "mailer"
weight: 10
description: >
  Configuration for the mailer service
---

# _struct: config_

{{% dir name="smtp_server" type="string" default="" %}}
The hostname and port of the SMTP server. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/mailer/mailer.go#L54)
{{< highlight toml >}}
[http.services.mailer]
smtp_server = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="sender_login" type="string" default="" %}}
The email to be used to send mails. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/mailer/mailer.go#L55)
{{< highlight toml >}}
[http.services.mailer]
sender_login = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="sender_password" type="string" default="" %}}
The sender's password. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/mailer/mailer.go#L56)
{{< highlight toml >}}
[http.services.mailer]
sender_password = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="disable_auth" type="bool" default=false %}}
Whether to disable SMTP auth. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/mailer/mailer.go#L57)
{{< highlight toml >}}
[http.services.mailer]
disable_auth = false
{{< /highlight >}}
{{% /dir %}}

