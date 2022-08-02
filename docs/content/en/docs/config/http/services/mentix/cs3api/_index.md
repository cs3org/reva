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

{{% dir name="endpoint" type="string" default="/cs3" %}}
The endpoint where the mesh data can be queried.
{{< highlight toml >}}
[http.services.mentix.exporters.cs3api]
endpoint = "/data"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="is_protected" type="bool" default="false" %}}
Whether the endpoint requires authentication.
{{< highlight toml >}}
[http.services.mentix.exporters.cs3api]
is_protected = true
{{< /highlight >}}
{{% /dir %}}

{{% dir name="enabled_connectors" type="[]string" default="*" %}}
A list of all enabled connectors for the exporter.
{{< highlight toml >}}
[http.services.mentix.exporters.cs3api]
enabled_connectors = ["gocdb"]
{{< /highlight >}}
{{% /dir %}}

{{% dir name="elevated_service_types" type="[]string" default="[GATEWAY,OCM,WEBDAV]" %}}
When processing additional endpoints of a service, any service type listed here will be elevated to a standalone service.
{{< highlight toml >}}
[http.services.mentix.exporters.cs3api]
elevated_service_types = ["METRICS", "WEBDAV"]
{{< /highlight >}}
{{% /dir %}}

