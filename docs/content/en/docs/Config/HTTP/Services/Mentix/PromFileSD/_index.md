---
title: "prom_filesd"
linkTitle: "prom_filesd"
weight: 10
description: >
    Configuration for the Prometheus File SD exporter of the Mentix service
---

{{% pageinfo %}}
When using the Prometheus File SD exporter, the output filename has to be configured first.
{{% /pageinfo %}}

{{% dir name="output_file" type="string" default="" %}}
The target filename of the generated Prometheus File SD scrape config.
{{< highlight toml >}}
[http.services.mentix.prom_filesd]
output_file = "/var/shared/prometheus/sciencemesh.json"
{{< /highlight >}}
{{% /dir %}}
