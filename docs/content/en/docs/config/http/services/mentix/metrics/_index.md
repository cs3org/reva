---
title: "metrics"
linkTitle: "metrics"
weight: 10
description: >
    Configuration for the Metrics exporter of the Mentix service
---

{{% pageinfo %}}
The Metrics exporter exposes site-specific metrics through Prometheus.
{{% /pageinfo %}}

{{% dir name="enabled_connectors" type="[]string" default="*" %}}
A list of all enabled connectors for the exporter.
{{< highlight toml >}}
[http.services.mentix.exporters.metrics]
enabled_connectors = ["gocdb"]
{{< /highlight >}}
{{% /dir %}}
