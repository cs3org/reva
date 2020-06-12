---
title: "GRPC"
linkTitle: "GRPC"
weight: 10
description: >
  Configuration reference for GRPC
---

{{% dir name="network" type="string" default="tcp" %}}
Specifies the network type. 
{{< highlight toml >}}
[grpc]
network = "tcp"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="address" type="string" default="localhost:9999" %}}
Specifies the bind address interface.
{{< highlight toml >}}
[grpc]
address = "0.0.0.0:9999"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="enabled_services" type="[string]" default="[]" %}}
List of GRPC services to be enabled.
{{< highlight toml >}}
[grpc]
enabled_services = ["helloworld"]
{{< /highlight >}}
{{% /dir %}}

{{% dir name="enabled_middlewares" type="[string]" default="[]" %}}
List of GRPC middlewares to be enabled.
{{< highlight toml >}}
[grpc]
enabled_interceptors = ["auth"]
{{< /highlight >}}
{{% /dir %}}
