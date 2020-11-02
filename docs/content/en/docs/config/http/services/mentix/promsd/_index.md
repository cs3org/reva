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

{{% dir name="metrics_output_file" type="string" default="" %}}
The target filename of the generated Prometheus File SD scrape config for metrics.
{{< highlight toml >}}
[http.services.mentix.exporters.promsd]
metrics_output_file = "/var/shared/prometheus/sciencemesh.json"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="blackbox_output_file" type="string" default="" %}}
The target filename of the generated Prometheus File SD scrape config for the blackbox exporter.
{{< highlight toml >}}
[http.services.mentix.exporters.promsd]
blackbox_output_file = "/var/shared/prometheus/blackbox.json"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="enabled_connectors" type="[]string" default="*" %}}
A list of all enabled connectors for the exporter.
{{< highlight toml >}}
[http.services.mentix.exporters.promsd]
enabled_connectors = ["gocdb"]
{{< /highlight >}}
{{% /dir %}}
