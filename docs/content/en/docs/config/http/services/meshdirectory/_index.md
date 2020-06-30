---
title: "meshdirectory"
linkTitle: "meshdirectory"
weight: 10
description: >
  Configuration for the Mesh Directory service
---

{{% dir name="static" type="string" default="static" %}}
Path to a static directory containing a UI frontend for the service.
This directory must contain an **index.html** file at least.
{{< highlight toml >}}
[http.services.meshdirectory]
static = "/path/to/static"
{{< /highlight >}}
{{% /dir %}}
