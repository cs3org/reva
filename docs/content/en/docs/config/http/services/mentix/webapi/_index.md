---
title: "webapi"
linkTitle: "webapi"
weight: 10
description: >
    Configuration for the WebAPI of the Mentix service
---

{{% pageinfo %}}
The WebAPI exporter supports multiple endpoints for exporting data. As there currently is only one such endpoint, the WebAPI settings should not be modified.
{{% /pageinfo %}}

{{% dir name="endpoint" type="string" default="/" %}}
The endpoint where the mesh data can be queried.
{{< highlight toml >}}
[http.services.mentix.webapi]
endpoint = "data"
{{< /highlight >}}
{{% /dir %}}
