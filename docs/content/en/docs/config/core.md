---
title: "Core"
linkTitle: "Core"
weight: 5
description: >
  Directives to tweak the running Reva process.
---

{{% dir name="max_cpus" type="string" default="100%" %}}
Set the number of cpus the running process will use.
You can use a percentage (70%) or a number of cpus (6) in the value.

{{< highlight toml >}}
[core]
max_cpus = "50%"
{{< /highlight >}}

{{% /dir %}}

{{% dir name="tracing_enabled" type="boolean" default="false" %}}
Enables tracing of requests. The only available tracer for the time being is Jaegger.

{{< highlight toml >}}
[core]
tracing_enabled = true
{{< /highlight >}}

{{% /dir %}}

{{% dir name="tracing_endpoint" type="string" default="localhost:6831" %}}
Address of the tracing server.
{{< highlight toml >}}
[core]
tracing_endpoint = "mytracer.example.org"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="tracing_collector" type="string" default="http://localhost:14268/api/traces" %}}
Endpoint of the request collector.
{{< highlight toml >}}
[core]
tracing_collector = "http://mytracer.example.org:14268/api/traces"
{{< /highlight >}}
{{% /dir %}}
