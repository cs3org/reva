---
title: "ocmshareprovider-driver-nextcloud"
linkTitle: "ocmshareprovider-driver-nextcloud"
weight: 10
description: >
  Configuration for the Nextcloud driver for the OCM Share Provider service
---

{{% pageinfo %}}
The Nextcloud driver for the OCM Share provider needs some configuration values.
You can use this driver for Nextcloud EFSS as well as for ownCloud 10 EFSS as the
Share provider backend. In both cases, when you create an OCM share (whether through
your EFSS GUI or through Reva CLI, the share stored as a sent share in your EFSS database,
and when you receive a share, whether through Reva's OCM core module, or through your EFSS
native OCM API, it will be listed as a received share in your EFSS database.
{{% /pageinfo %}}

{{% dir name="webdav_host" type="string" default="" %}}
The WebDAV interface of your EFSS, through which others can access the resources
of shares you send.
{{< highlight toml >}}
[grpc.services.ocmshareprovider.drivers.nextcloud]
webdav_host = "https://cloud.space.academy/"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="endpoint" type="string" default="" %}}
The URL of the ScienceMesh app on your EFSS instance.
{{< highlight toml >}}
[grpc.services.ocmshareprovider.drivers.nextcloud]
endpoint = "https://nc1.docker/index.php/apps/sciencemesh/"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="shared_secret" type="string" default="" %}}
The shared secret between this Reva instance and the ScienceMesh app on your EFSS instance.
{{< highlight toml >}}
[grpc.services.ocmshareprovider.drivers.nextcloud]
shared_secret = "shared-secret-1"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="mock_http" type="boolean" default="false" %}}
Used in the integration tests, leave this set to false.
{{< highlight toml >}}
[grpc.services.ocmshareprovider.drivers.nextcloud]
mock_http = false
{{< /highlight >}}
{{% /dir %}}
