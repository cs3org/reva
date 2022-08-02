---
title: "promsd"
linkTitle: "promsd"
weight: 10
description: >
    Configuration for the Prometheus SD exporter of the Mentix service
---

{{% pageinfo %}}
When using the Prometheus SD exporter, the output filenames have to be configured first.
{{% /pageinfo %}}

{{% dir name="output_path" type="string" default="" %}}
The target path of the generated Prometheus File SD scrape configs for metrics.
{{< highlight toml >}}
[http.services.mentix.exporters.promsd]
output_path = "/var/shared/prometheus/sciencemesh"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="enabled_connectors" type="[]string" default="*" %}}
A list of all enabled connectors for the exporter.
{{< highlight toml >}}
[http.services.mentix.exporters.promsd]
enabled_connectors = ["gocdb"]
{{< /highlight >}}
{{% /dir %}}
