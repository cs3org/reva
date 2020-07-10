---
title: "webapi"
linkTitle: "webapi"
weight: 10
description: >
    Configuration for the WebAPI of the Mentix service
---

{{% pageinfo %}}
The WebAPI exporter exposes the _plain_ Mentix data via an HTTP endpoint.
{{% /pageinfo %}}

{{% dir name="endpoint" type="string" default="/" %}}
The endpoint where the mesh data can be queried.
{{< highlight toml >}}
[http.services.mentix.webapi]
endpoint = "/data"
{{< /highlight >}}
{{% /dir %}}
