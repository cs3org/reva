---
title: "storageprovider"
linkTitle: "storageprovider"
weight: 10
description: >
  Configuration for the Storage Provider service
---

# _struct: config_

{{% dir name="driver" type="string" default="local" %}}
The storage driver to be used. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L51)
{{< highlight toml >}}
[grpc.services.storageprovider]
driver = "local"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="drivers" type="map[string]map[string]interface{}" default="docs/config/packages/storage/fs" %}}
The configuration for the storage driver. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L53)
{{< highlight toml >}}
[grpc.services.storageprovider.drivers]
"[docs/config/packages/storage/fs]({{< ref "docs/config/packages/storage/fs" >}})"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="path_wrapper" type="string" default="/var/tmp/reva/" %}}
Path wrapper. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L55)
{{< highlight toml >}}
[grpc.services.storageprovider]
path_wrapper = "/var/tmp/reva/"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="path_wrappers" type="map[string]map[string]interface{}" default=nil %}}
The configuration for the path wrapper. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L58)
{{< highlight toml >}}
[grpc.services.storageprovider]
path_wrappers = nil
{{< /highlight >}}
{{% /dir %}}

{{% dir name="tmp_folder" type="string" default="/var/tmp" %}}
Path to temporary folder. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L59)
{{< highlight toml >}}
[grpc.services.storageprovider]
tmp_folder = "/var/tmp"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="data_server_url" type="string" default="http://localhost/data" %}}
The URL for the data server. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L60)
{{< highlight toml >}}
[grpc.services.storageprovider]
data_server_url = "http://localhost/data"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="expose_data_server" type="bool" default=false %}}
Whether to expose data server. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L61)
{{< highlight toml >}}
[grpc.services.storageprovider]
expose_data_server = false
{{< /highlight >}}
{{% /dir %}}

{{% dir name="enable_home_creation" type="bool" default=false %}}
Whether to enable home creation. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L64)
{{< highlight toml >}}
[grpc.services.storageprovider]
enable_home_creation = false
{{< /highlight >}}
{{% /dir %}}

{{% dir name="disable_tus" type="bool" default=false %}}
Whether to disable TUS uploads. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L65)
{{< highlight toml >}}
[grpc.services.storageprovider]
disable_tus = false
{{< /highlight >}}
{{% /dir %}}

{{% dir name="available_checksums" type="map[string]uint32" default=nil %}}
List of available checksums. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L66)
{{< highlight toml >}}
[grpc.services.storageprovider]
available_checksums = nil
{{< /highlight >}}
{{% /dir %}}

