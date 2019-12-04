---
title: "prometheus"
linkTitle: "prometheus"
weight: 10
description: >
  Configuration for the Prometheus service
---

{{% pageinfo %}}
TODO
{{% /pageinfo %}}

{{% dir name="prefix" type="string" default="metrics" %}}
Where the HTTP service is exposed.
{{< highlight toml >}}
[http.services.prometheus]
prefix = "/"
{{< /highlight >}}
{{% /dir %}}

