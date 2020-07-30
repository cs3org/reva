---
title: "sysinfo"
linkTitle: "sysinfo"
weight: 10
description: >
  Configuration for the system information service
---

{{% dir name="prefix" type="string" default="sysinfo" %}}
Endpoint of the system information service.
{{< highlight toml >}}
[http.services.sysinfo]
prefix = "/sysinfo"
{{< /highlight >}}
{{% /dir %}}
