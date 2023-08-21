---
title: "storageprovider"
linkTitle: "storageprovider"
weight: 10
description: >
  Configuration for the storageprovider service
---

# _struct: config_

{{% dir name="driver" type="string" default="localhome" %}}
The storage driver to be used. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L62)
{{< highlight toml >}}
[grpc.services.storageprovider]
driver = "localhome"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="drivers" type="map[string]map[string]interface{}" default="localhome" %}}
 [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L63)
{{< highlight toml >}}
[grpc.services.storageprovider.drivers.localhome]
root = "/var/tmp/reva/"
share_folder = "/MyShares"
user_layout = "{{.Username}}"

{{< /highlight >}}
{{% /dir %}}

{{% dir name="data_server_url" type="string" default="http://localhost/data" %}}
The URL for the data server. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L64)
{{< highlight toml >}}
[grpc.services.storageprovider]
data_server_url = "http://localhost/data"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="expose_data_server" type="bool" default=false %}}
Whether to expose data server. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L65)
{{< highlight toml >}}
[grpc.services.storageprovider]
expose_data_server = false
{{< /highlight >}}
{{% /dir %}}

{{% dir name="available_checksums" type="map[string]uint32" default=nil %}}
List of available checksums. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L66)
{{< highlight toml >}}
[grpc.services.storageprovider]
available_checksums = nil
{{< /highlight >}}
{{% /dir %}}

{{% dir name="custom_mimetypes_json" type="string" default="nil" %}}
An optional mapping file with the list of supported custom file extensions and corresponding mime types. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L67)
{{< highlight toml >}}
[grpc.services.storageprovider]
custom_mimetypes_json = "nil"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="upload_expiration" type="int64" default=0 %}}
Duration for how long uploads will be valid. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L69)
{{< highlight toml >}}
[grpc.services.storageprovider]
upload_expiration = 0
{{< /highlight >}}
{{% /dir %}}

{{% dir name="events" type="eventconfig" default=0 %}}
Event stream configuration [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L70)
{{< highlight toml >}}
[grpc.services.storageprovider]
events = 0
{{< /highlight >}}
{{% /dir %}}

# _struct: eventconfig_

{{% dir name="nats_address" type="string" default="address of the nats server" %}}
 [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L74)
{{< highlight toml >}}
[grpc.services.storageprovider]
nats_address = "address of the nats server"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="nats_clusterid" type="string" default="clusterid of the nats server" %}}
 [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L75)
{{< highlight toml >}}
[grpc.services.storageprovider]
nats_clusterid = "clusterid of the nats server"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="tls_insecure" type="bool" default=Whether to verify the server TLS certificates. %}}
 [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L76)
{{< highlight toml >}}
[grpc.services.storageprovider]
tls_insecure = Whether to verify the server TLS certificates.
{{< /highlight >}}
{{% /dir %}}

{{% dir name="tls_root_ca_cert" type="string" default="The root CA certificate used to validate the server's TLS certificate." %}}
 [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L77)
{{< highlight toml >}}
[grpc.services.storageprovider]
tls_root_ca_cert = "The root CA certificate used to validate the server's TLS certificate."
{{< /highlight >}}
{{% /dir %}}

{{% dir name="nats_enable_tls" type="bool" default=events tls switch %}}
 [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/storageprovider/storageprovider.go#L78)
{{< highlight toml >}}
[grpc.services.storageprovider]
nats_enable_tls = events tls switch
{{< /highlight >}}
{{% /dir %}}

