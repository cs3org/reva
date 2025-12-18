---
title: "signed_url"
linkTitle: "signed_url"
weight: 10
description: >
  Configuration for the signed_url service
---

# _struct: Config_

{{% dir name="max_expiry_seconds" type="int" default=nil %}}
 Default: one day [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/interceptors/auth/signed_url/strategy/signed_url/signed_url.go#L53)
{{< highlight toml >}}
[http.interceptors.auth.signed_url.strategy.signed_url]
max_expiry_seconds = nil
{{< /highlight >}}
{{% /dir %}}

