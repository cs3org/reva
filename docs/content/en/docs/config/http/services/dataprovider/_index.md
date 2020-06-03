---
title: "dataprovider"
linkTitle: "dataprovider"
weight: 10
description: >
  Configuration for the DataProvider service
---

{{% dir name="prefix" type="string" default="data" %}}
The prefix to be used for this HTTP service
{{< highlight toml >}}
[http.services.dataprovider]
prefix = "data"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="driver" type="string" default="json" %}}
The storage driver to be used.
{{< highlight toml >}}
[http.services.dataprovider]
driver = "json"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="drivers" type="map[string]map[string]interface{}" default="[FS]({{< relref "docs/config/packages/storage/fs/" >}})" %}}
The configuration for the storage driver
{{< highlight toml >}}
[http.services.dataprovider]
drivers = "[FS]({{< relref "docs/config/packages/storage/fs/" >}})"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="disable_tus" type="bool" default="false" %}}
Whether to disable TUS uploads.
{{< highlight toml >}}
[http.services.dataprovider]
disable_tus = "false"
{{< /highlight >}}
{{% /dir %}}

