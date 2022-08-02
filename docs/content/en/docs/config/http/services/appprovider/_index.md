---
title: "appprovider"
linkTitle: "appprovider"
weight: 10
description: >
  Configuration for the appprovider service
---

# _struct: Config_

{{% dir name="insecure" type="bool" default=false %}}
Whether to skip certificate checks when sending requests. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/appprovider/appprovider.go#L54)
{{< highlight toml >}}
[http.services.appprovider]
insecure = false
{{< /highlight >}}
{{% /dir %}}

