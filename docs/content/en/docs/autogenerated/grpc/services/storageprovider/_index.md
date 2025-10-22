---
title: "storageprovider"
linkTitle: "storageprovider"
weight: 10
description: >
  Configuration for the storageprovider service
---

# _struct: config_

{{% dir name="mount_path" type="string" default="/" %}}
The path where the file system would be mounted. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L74)
{{< highlight toml >}}
[grpc.services.storageprovider]
mount_path = "/"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="mount_id" type="string" default="-" %}}
The ID of the mounted file system. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L75)
{{< highlight toml >}}
[grpc.services.storageprovider]
mount_id = "-"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="driver" type="string" default="localhome" %}}
The storage driver to be used. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L76)
{{< highlight toml >}}
[grpc.services.storageprovider]
driver = "localhome"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="drivers" type="map[string]map[string]interface{}" default="localhome" %}}
 [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L77)
{{< highlight toml >}}
[grpc.services.storageprovider.drivers.localhome]
root = "/var/tmp/reva/"
share_folder = "/MyShares"
user_layout = "{{.Username}}"

{{< /highlight >}}
{{% /dir %}}

{{% dir name="data_server_url" type="string" default="http://localhost/data" %}}
The URL for the data server. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L78)
{{< highlight toml >}}
[grpc.services.storageprovider]
data_server_url = "http://localhost/data"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="expose_data_server" type="bool" default=false %}}
Whether to expose data server. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L79)
{{< highlight toml >}}
[grpc.services.storageprovider]
expose_data_server = false
{{< /highlight >}}
{{% /dir %}}

{{% dir name="available_checksums" type="map[string]uint32" default=nil %}}
List of available checksums. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L80)
{{< highlight toml >}}
[grpc.services.storageprovider]
available_checksums = nil
{{< /highlight >}}
{{% /dir %}}

{{% dir name="custom_mime_types_json" type="string" default="nil" %}}
An optional mapping file with the list of supported custom file extensions and corresponding mime types. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L81)
{{< highlight toml >}}
[grpc.services.storageprovider]
custom_mime_types_json = "nil"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="provides_space_type" type="string" default="nil" %}}
Defines which type of spaces this storage provider provides (e.g. home, project, ...). [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L87)
{{< highlight toml >}}
[grpc.services.storageprovider]
provides_space_type = "nil"
{{< /highlight >}}
{{% /dir %}}

