---
title: "dataprovider"
linkTitle: "dataprovider"
weight: 10
description: >
  Configuration for the DataProvider service
---

# _struct: config_

{{% dir name="prefix" type="string" default="data" %}}
The prefix to be used for this HTTP service [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/dataprovider/dataprovider.go#L40)
{{< highlight toml >}}
[http.services.dataprovider]
prefix = "data"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="driver" type="string" default="localhome" %}}
The storage driver to be used. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/dataprovider/dataprovider.go#L41)
{{< highlight toml >}}
[http.services.dataprovider]
driver = "localhome"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="drivers" type="map[string]map[string]interface{}" default="localhome" %}}
The configuration for the storage driver [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/dataprovider/dataprovider.go#L42)
{{< highlight toml >}}
[http.services.dataprovider.drivers.localhome]
root = "/var/tmp/reva/"
share_folder = "/MyShares"
user_layout = "{{.Username}}"

{{< /highlight >}}
{{% /dir %}}

{{% dir name="data_txs" type="map[string]map[string]interface{}" default="simple" %}}
The configuration for the data tx protocols [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/dataprovider/dataprovider.go#L43)
{{< highlight toml >}}
[http.services.dataprovider.data_txs.simple]

{{< /highlight >}}
{{% /dir %}}

