---
title: "dataprovider"
linkTitle: "dataprovider"
weight: 10
description: >
  Configuration for the DataProvider service
---

struct config

{{% dir name="prefix" type="string" default="data" %}}
The prefix to be used for this HTTP service
{{< highlight toml >}}
[http.services.dataprovider]
prefix = "data"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="driver" type="string" default="local" %}}
The storage driver to be used.
{{< highlight toml >}}
[http.services.dataprovider]
driver = "local"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="drivers" type="map[string]map[string]interface{}" default="docs/config/packages/storage/fs" %}}
The configuration for the storage driver
{{< highlight toml >}}
[http.services.dataprovider.drivers]
"[docs/config/packages/storage/fs]({{< ref "docs/config/packages/storage/fs" >}})"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="disable_tus" type="bool" default="false" %}}
Whether to disable TUS uploads.
{{< highlight toml >}}
[http.services.dataprovider]
disable_tus = "false"
{{< /highlight >}}
{{% /dir %}}

