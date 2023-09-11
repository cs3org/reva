---
title: "ocmprovider"
linkTitle: "ocmprovider"
weight: 10
description: >
  Configuration for the ocmprovider service
---

# _struct: config_

{{% dir name="ocm_prefix" type="string" default="ocm" %}}
The prefix URL where the OCM API is served. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/ocmprovider/ocmprovider.go#L39)
{{< highlight toml >}}
[http.services.ocmprovider]
ocm_prefix = "ocm"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="endpoint" type="string" default="This host's URL. If it's not configured, it is assumed OCM is not available." %}}
 [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/ocmprovider/ocmprovider.go#L40)
{{< highlight toml >}}
[http.services.ocmprovider]
endpoint = "This host's URL. If it's not configured, it is assumed OCM is not available."
{{< /highlight >}}
{{% /dir %}}

{{% dir name="provider" type="string" default="reva" %}}
A friendly name that defines this service. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/ocmprovider/ocmprovider.go#L41)
{{< highlight toml >}}
[http.services.ocmprovider]
provider = "reva"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="webdav_root" type="string" default="/remote.php/dav/ocm" %}}
The root URL of the WebDAV endpoint to serve OCM shares. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/ocmprovider/ocmprovider.go#L42)
{{< highlight toml >}}
[http.services.ocmprovider]
webdav_root = "/remote.php/dav/ocm"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="webapp_root" type="string" default="/external/sciencemesh" %}}
The root URL to serve Web apps via OCM. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/ocmprovider/ocmprovider.go#L43)
{{< highlight toml >}}
[http.services.ocmprovider]
webapp_root = "/external/sciencemesh"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="enable_webapp" type="bool" default=false %}}
Whether web apps are enabled in OCM shares. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/ocmprovider/ocmprovider.go#L44)
{{< highlight toml >}}
[http.services.ocmprovider]
enable_webapp = false
{{< /highlight >}}
{{% /dir %}}

{{% dir name="enable_datatx" type="bool" default=false %}}
Whether data transfers are enabled in OCM shares. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/ocmprovider/ocmprovider.go#L45)
{{< highlight toml >}}
[http.services.ocmprovider]
enable_datatx = false
{{< /highlight >}}
{{% /dir %}}

