---
title: "datagateway"
linkTitle: "datagateway"
weight: 10
description: >
  Configuration for the DataGateway service
---

# _struct: config_

{{% dir name="insecure" type="bool" default=false %}}
Whether to skip certificate checks when sending requests. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/datagateway/datagateway.go#L62)
{{< highlight toml >}}
[http.services.datagateway]
insecure = false
{{< /highlight >}}
{{% /dir %}}

