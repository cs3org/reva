---
title: "siteloc"
linkTitle: "siteloc"
weight: 10
description: >
    Configuration for the Site Locations exporter of the Mentix service
---

{{% pageinfo %}}
The Site Locations exporter exposes location information of all sites to be consumed by Grafana via an HTTP endpoint.
{{% /pageinfo %}}

{{% dir name="endpoint" type="string" default="/" %}}
The endpoint where the locations data can be queried.
{{< highlight toml >}}
[http.services.mentix.exporters.siteloc]
endpoint = "/loc"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="enabled_connectors" type="[]string" default="*" %}}
A list of all enabled connectors for the exporter.
{{< highlight toml >}}
[http.services.mentix.exporters.siteloc]
enabled_connectors = ["gocdb"]
{{< /highlight >}}
{{% /dir %}}
