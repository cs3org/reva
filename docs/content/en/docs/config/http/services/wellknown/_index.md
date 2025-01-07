---
title: "wellknown"
linkTitle: "wellknown"
weight: 10
description: >
  Configuration for the HelloWorld service
---

# _struct: OcmProviderConfig_

{{% dir name="ocm_prefix" type="string" default="ocm" %}}
The prefix URL where the OCM API is served. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/wellknown/ocm.go#L33)
{{< highlight toml >}}
[http.services.wellknown]
ocm_prefix = "ocm"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="endpoint" type="string" default="This host's full URL. If it's not configured, it is assumed OCM is not available." %}}
 [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/wellknown/ocm.go#L34)
{{< highlight toml >}}
[http.services.wellknown]
endpoint = "This host's full URL. If it's not configured, it is assumed OCM is not available."
{{< /highlight >}}
{{% /dir %}}

{{% dir name="provider" type="string" default="reva" %}}
A friendly name that defines this service. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/wellknown/ocm.go#L35)
{{< highlight toml >}}
[http.services.wellknown]
provider = "reva"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="webdav_root" type="string" default="/remote.php/dav/ocm" %}}
The root URL of the WebDAV endpoint to serve OCM shares. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/wellknown/ocm.go#L36)
{{< highlight toml >}}
[http.services.wellknown]
webdav_root = "/remote.php/dav/ocm"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="webapp_root" type="string" default="/external/sciencemesh" %}}
The root URL to serve Web apps via OCM. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/wellknown/ocm.go#L37)
{{< highlight toml >}}
[http.services.wellknown]
webapp_root = "/external/sciencemesh"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="enable_webapp" type="bool" default=false %}}
Whether web apps are enabled in OCM shares. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/wellknown/ocm.go#L38)
{{< highlight toml >}}
[http.services.wellknown]
enable_webapp = false
{{< /highlight >}}
{{% /dir %}}

{{% dir name="enable_datatx" type="bool" default=false %}}
Whether data transfers are enabled in OCM shares. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/wellknown/ocm.go#L39)
{{< highlight toml >}}
[http.services.wellknown]
enable_datatx = false
{{< /highlight >}}
{{% /dir %}}

