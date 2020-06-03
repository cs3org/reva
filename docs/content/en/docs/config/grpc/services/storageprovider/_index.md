---
title: "storageprovider"
linkTitle: "storageprovider"
weight: 10
description: >
  Configuration for the Storage Provider service
---

{{% dir name="driver" type="string" default="json" %}}
The storage driver to be used.
{{< highlight toml >}}
[grpc.services.storageprovider]
driver = "json"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="drivers" type="map[string]map[string]interface{}" default="[FS]({{< relref "docs/config/packages/storage/fs/" >}})" %}}
The configuration for the storage driver.
{{< highlight toml >}}
[grpc.services.storageprovider]
drivers = "[FS]({{< relref "docs/config/packages/storage/fs/" >}})"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="path_wrapper" type="string" default="" %}}
Path wrapper.
{{< highlight toml >}}
[grpc.services.storageprovider]
path_wrapper = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="path_wrappers" type="map[string]map[string]interface{}" default="" %}}
The configuration for the path wrapper.
{{< highlight toml >}}
[grpc.services.storageprovider]
path_wrappers = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="tmp_folder" type="string" default="/var/tmp" %}}
Path to temporary folder.
{{< highlight toml >}}
[grpc.services.storageprovider]
tmp_folder = "/var/tmp"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="data_server_url" type="string" default="http://localhost/data" %}}
 The URL for the data server.
{{< highlight toml >}}
[grpc.services.storageprovider]
data_server_url = "http://localhost/data"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="expose_data_server" type="bool" default="false" %}}
Whether to expose data server.
{{< highlight toml >}}
[grpc.services.storageprovider]
expose_data_server = "false"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="enable_home_creation" type="bool" default="false" %}}
Whether to enable home creation.
{{< highlight toml >}}
[grpc.services.storageprovider]
enable_home_creation = "false"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="disable_tus" type="bool" default="false" %}}
Whether to disable TUS uploads.
{{< highlight toml >}}
[grpc.services.storageprovider]
disable_tus = "false"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="available_checksums" type="map[string]uint32" default="false" %}}
List of available checksums.
{{< highlight toml >}}
[grpc.services.storageprovider]
available_checksums = "false"
{{< /highlight >}}
{{% /dir %}}

