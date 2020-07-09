---
title: "cs3api"
linkTitle: "cs3api"
weight: 10
description: >
    Configuration for the CS3API of the Mentix service
---

{{% pageinfo %}}
The CS3API exporter exposes Mentix data in a format that is compliant with the CS3API `ProviderInfo` structure via an HTTP endpoint.
{{% /pageinfo %}}

{{% dir name="endpoint" type="string" default="/" %}}
The endpoint where the mesh data can be queried.
{{< highlight toml >}}
[http.services.mentix.cs3api]
endpoint = "/data"
{{< /highlight >}}
{{% /dir %}}
