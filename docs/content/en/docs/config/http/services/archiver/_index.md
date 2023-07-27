---
title: "archiver"
linkTitle: "archiver"
weight: 10
description: >
  Configuration for the archiver service
---

# _struct: Config_

{{% dir name="insecure" type="bool" default=false %}}
Whether to skip certificate checks when sending requests. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/archiver/handler.go#L62)
{{< highlight toml >}}
[http.services.archiver]
insecure = false
{{< /highlight >}}
{{% /dir %}}

