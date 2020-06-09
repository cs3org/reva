---
title: "ocdav"
linkTitle: "ocdav"
weight: 10
description: >
  Configuration for the OCDav service
---

{{% pageinfo %}}
TODO
{{% /pageinfo %}}

{{% dir name="prefix" type="string" default=".well-known" %}}
Where the HTTP service is exposed.
{{< highlight toml >}}
[http.services.wellknown]
prefix = "/"
{{< /highlight >}}
{{% /dir %}}

