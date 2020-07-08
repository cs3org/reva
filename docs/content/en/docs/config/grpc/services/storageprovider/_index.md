---
title: "storageprovider"
linkTitle: "storageprovider"
weight: 10
description: >
  Configuration for the Storage Provider service
---

# _struct: config_

{{% dir name="mount_path" type="string" default="/" %}}
The path where the file system would be mounted. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L51)
{{< highlight toml >}}
[grpc.services.storageprovider]
mount_path = "/"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="mount_id" type="string" default="-" %}}
The ID of the mounted file system. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L52)
{{< highlight toml >}}
[grpc.services.storageprovider]
mount_id = "-"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="driver" type="string" default="localhome" %}}
The storage driver to be used. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L53)
{{< highlight toml >}}
[grpc.services.storageprovider]
driver = "localhome"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="drivers" type="map[string]map[string]interface{}" default="docs/config/packages/storage/fs" %}}
 [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L54)
{{< highlight toml >}}
[grpc.services.storageprovider.drivers]
"[docs/config/packages/storage/fs]({{< ref "docs/config/packages/storage/fs" >}})"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="tmp_folder" type="string" default="/var/tmp" %}}
Path to temporary folder. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L55)
{{< highlight toml >}}
[grpc.services.storageprovider]
tmp_folder = "/var/tmp"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="data_server_url" type="string" default="http://localhost/data" %}}
The URL for the data server. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L56)
{{< highlight toml >}}
[grpc.services.storageprovider]
data_server_url = "http://localhost/data"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="expose_data_server" type="bool" default=false %}}
Whether to expose data server. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L57)
{{< highlight toml >}}
[grpc.services.storageprovider]
expose_data_server = false
{{< /highlight >}}
{{% /dir %}}

{{% dir name="disable_tus" type="bool" default=false %}}
Whether to disable TUS uploads. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L58)
{{< highlight toml >}}
[grpc.services.storageprovider]
disable_tus = false
{{< /highlight >}}
{{% /dir %}}

{{% dir name="available_checksums" type="map[string]uint32" default=nil %}}
List of available checksums. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L59)
{{< highlight toml >}}
[grpc.services.storageprovider]
available_checksums = nil
{{< /highlight >}}
{{% /dir %}}

