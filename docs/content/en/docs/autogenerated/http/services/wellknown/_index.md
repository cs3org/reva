---
title: "wellknown"
linkTitle: "wellknown"
weight: 10
description: >
  Configuration for the wellknown service
---

# _struct: OcmProviderConfig_

{{% dir name="ocm_prefix" type="string" default="ocm" %}}
The prefix URL where the OCM API is served. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/wellknown/ocm.go#L55)
{{< highlight toml >}}
[http.services.wellknown]
ocm_prefix = "ocm"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="endpoint" type="string" default="This host's full URL. If it's not configured, it is assumed OCM is not available." %}}
 [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/wellknown/ocm.go#L56)
{{< highlight toml >}}
[http.services.wellknown]
endpoint = "This host's full URL. If it's not configured, it is assumed OCM is not available."
{{< /highlight >}}
{{% /dir %}}

{{% dir name="provider" type="string" default="reva" %}}
A friendly name that defines this service. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/wellknown/ocm.go#L57)
{{< highlight toml >}}
[http.services.wellknown]
provider = "reva"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="webdav_root" type="string" default="/remote.php/dav/ocm" %}}
The root URL of the WebDAV endpoint to serve OCM shares. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/wellknown/ocm.go#L58)
{{< highlight toml >}}
[http.services.wellknown]
webdav_root = "/remote.php/dav/ocm"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="invite_accept_dialog" type="string" default="/open-cloud-mesh/accept-invite" %}}
The frontend URL where to land when receiving an invitation [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/wellknown/ocm.go#L59)
{{< highlight toml >}}
[http.services.wellknown]
invite_accept_dialog = "/open-cloud-mesh/accept-invite"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="enable_webapp" type="bool" default=false %}}
Advertise webapp receive in discovery only. Distinct from offer_webapp. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/wellknown/ocm.go#L60)
{{< highlight toml >}}
[http.services.wellknown]
enable_webapp = false
{{< /highlight >}}
{{% /dir %}}

{{% dir name="enable_embedded" type="bool" default=false %}}
Whether embedded shares are enabled in OCM shares. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/wellknown/ocm.go#L61)
{{< highlight toml >}}
[http.services.wellknown]
enable_embedded = false
{{< /highlight >}}
{{% /dir %}}

{{% dir name="enable_code_flow" type="bool" default=false %}}
Whether code-flow token exchange is enabled in OCM shares. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/wellknown/ocm.go#L62)
{{< highlight toml >}}
[http.services.wellknown]
enable_code_flow = false
{{< /highlight >}}
{{% /dir %}}

{{% dir name="reject" type="string" default="Admission of received webapp must-use-mfa offers: reject or off. Off skips the check" %}}
 nothing is advertised. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/wellknown/ocm.go#L63)
{{< highlight toml >}}
[http.services.wellknown]
reject = "Admission of received webapp must-use-mfa offers: reject or off. Off skips the check"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="signing_key" type="string" default="" %}}
Path to the PEM file of the Ed25519 private key for OCM HTTP signatures. If it's not configured, no public key is published. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/wellknown/ocm.go#L64)
{{< highlight toml >}}
[http.services.wellknown]
signing_key = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="http_sig_mode" type="string" default="permissive" %}}
permissive or strict. Strict lists the must-use-http-sig criterion and needs signing_key. Use the same mode in the ocm service. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/wellknown/ocm.go#L65)
{{< highlight toml >}}
[http.services.wellknown]
http_sig_mode = "permissive"
{{< /highlight >}}
{{% /dir %}}

