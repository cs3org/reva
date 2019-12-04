---
title: "datagateway"
linkTitle: "datagateway"
weight: 10
description: >
  Configuration for the DataGateway service
---

{{% pageinfo %}}
TODO
{{% /pageinfo %}}

{{% dir name="prefix" type="string" default="datagateway" %}}
Where the HTTP service is exposed.
{{< highlight toml >}}
[http.services.datagateway]
prefix = "/"
{{< /highlight >}}
{{% /dir %}}

