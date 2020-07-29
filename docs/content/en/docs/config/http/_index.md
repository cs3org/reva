---
title: "HTTP"
linkTitle: "HTTP"
weight: 10
description: >
  Configuration reference for HTTP
---

{{% dir name="network" type="string" default="tcp" %}}
Specifies the network type.
{{< highlight toml >}}
[http]
network = "tcp"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="address" type="string" default="localhost" %}}
Specifies the bind address interface.
{{< highlight toml >}}
[http]
address = "0.0.0.0"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="enabled_services" type="[string]" default="[]" %}}
List of HTTP services to be enabled.
{{< highlight toml >}}
[http]
enabled_services = ["helloworld"]
{{< /highlight >}}
{{% /dir %}}

{{% dir name="enabled_middlewares" type="[string]" default="[]" %}}
List of HTTP middlewares to be enabled.
{{< highlight toml >}}
[http]
enabled_middlewares = ["cors"]
{{< /highlight >}}
{{% /dir %}}
