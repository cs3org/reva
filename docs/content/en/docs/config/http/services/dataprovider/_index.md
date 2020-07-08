---
title: "dataprovider"
linkTitle: "dataprovider"
weight: 10
description: >
  Configuration for the DataProvider service
---

# _struct: config_

{{% dir name="prefix" type="string" default="data" %}}
The prefix to be used for this HTTP service [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/dataprovider/dataprovider.go#L39)
{{< highlight toml >}}
[http.services.dataprovider]
prefix = "data"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="driver" type="string" default="localhome" %}}
The storage driver to be used. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/dataprovider/dataprovider.go#L40)
{{< highlight toml >}}
[http.services.dataprovider]
driver = "localhome"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="drivers" type="map[string]map[string]interface{}" default="docs/config/packages/storage/fs" %}}
The configuration for the storage driver [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/dataprovider/dataprovider.go#L41)
{{< highlight toml >}}
[http.services.dataprovider.drivers]
"[docs/config/packages/storage/fs]({{< ref "docs/config/packages/storage/fs" >}})"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="disable_tus" type="bool" default=false %}}
Whether to disable TUS uploads. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/dataprovider/dataprovider.go#L44)
{{< highlight toml >}}
[http.services.dataprovider]
disable_tus = false
{{< /highlight >}}
{{% /dir %}}

