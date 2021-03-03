---
title: "webapi"
linkTitle: "webapi"
weight: 10
description: >
    Configuration for the WebAPI of the Mentix service
---

{{% pageinfo %}}
The WebAPI of Mentix supports of mesh data via an HTTP endpoint.
{{% /pageinfo %}}

The WebAPI exporter exposes the _plain_ Mentix data via an HTTP endpoint.

{{% dir name="endpoint" type="string" default="/sites" %}}
The endpoint where the mesh data can be queried.
{{< highlight toml >}}
[http.services.mentix.exporters.webapi]
endpoint = "/data"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="is_protected" type="bool" default="false" %}}
Whether the endpoint requires authentication.
{{< highlight toml >}}
[http.services.mentix.exporters.webapi]
is_protected = true
{{< /highlight >}}
{{% /dir %}}

{{% dir name="enabled_connectors" type="[]string" default="*" %}}
A list of all enabled connectors for the exporter.
{{< highlight toml >}}
[http.services.mentix.exporters.webapi]
enabled_connectors = ["gocdb"]
{{< /highlight >}}
{{% /dir %}}
