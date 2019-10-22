---
title: "dataprovider"
linkTitle: "dataprovider"
weight: 10
description: >
  Configuration for the DataProvider service
---

{{% pageinfo %}}
TODO
{{% /pageinfo %}}

{{% dir name="prefix" type="string" default="dataprovider" %}}
Where the HTTP service is exposed.
{{< highlight toml >}}
[http.services.dataprovider]
prefix = "/"
{{< /highlight >}}
{{% /dir %}}

