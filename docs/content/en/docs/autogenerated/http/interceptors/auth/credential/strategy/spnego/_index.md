---
title: "spnego"
linkTitle: "spnego"
weight: 10
description: >
  Configuration for the spnego service
---

# _struct: config_

{{% dir name="auth_type" type="string" default="kerberos" %}}
The auth provider type the token is sent to. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/interceptors/auth/credential/strategy/spnego/spnego.go#L51)
{{< highlight toml >}}
[http.interceptors.auth.credential.strategy.spnego]
auth_type = "kerberos"
{{< /highlight >}}
{{% /dir %}}

