---
title: "auth"
linkTitle: "auth"
weight: 10
description: >
  Configuration for the auth service
---

# _struct: config_

{{% dir name="machine_secret" type="string" default="nil" %}}
Secret used for the gateway to authenticate a user when using a signed URL [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/interceptors/auth/auth.go#L74)
{{< highlight toml >}}
[http.interceptors.auth]
machine_secret = "nil"
{{< /highlight >}}
{{% /dir %}}

