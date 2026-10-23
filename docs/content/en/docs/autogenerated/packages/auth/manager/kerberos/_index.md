---
title: "kerberos"
linkTitle: "kerberos"
weight: 10
description: >
  Configuration for the kerberos service
---

# _struct: config_

{{% dir name="keytab" type="string" default="" %}}
Path to the service keytab. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/kerberos/kerberos.go#L73)
{{< highlight toml >}}
[auth.manager.kerberos]
keytab = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="service_principal" type="string" default="" %}}
The service principal name clients authenticate to. Empty accepts any principal in the keytab. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/kerberos/kerberos.go#L76)
{{< highlight toml >}}
[auth.manager.kerberos]
service_principal = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="realm" type="string" default="" %}}
The Kerberos realm to accept. Empty accepts any. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/kerberos/kerberos.go#L79)
{{< highlight toml >}}
[auth.manager.kerberos]
realm = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="user_claim" type="string" default="username" %}}
The user claim the username from the ticket is resolved against. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/kerberos/kerberos.go#L82)
{{< highlight toml >}}
[auth.manager.kerberos]
user_claim = "username"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="max_clock_skew_seconds" type="int" default=300 %}}
Maximum tolerated clock skew, in seconds. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/kerberos/kerberos.go#L86)
{{< highlight toml >}}
[auth.manager.kerberos]
max_clock_skew_seconds = 300
{{< /highlight >}}
{{% /dir %}}

