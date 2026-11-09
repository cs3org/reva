---
title: "ocmshareprovider"
linkTitle: "ocmshareprovider"
weight: 10
description: >
  Configuration for the ocmshareprovider service
---

# _struct: config_

{{% dir name="provider_domain" type="string" default="The same domain registered in the provider authorizer" %}}
 [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/ocmshareprovider/ocmshareprovider.go#L80)
{{< highlight toml >}}
[grpc.services.ocmshareprovider]
provider_domain = "The same domain registered in the provider authorizer"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="offer_webapp" type="bool" default=false %}}
Offer the webapp protocol on outbound OCM shares. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/ocmshareprovider/ocmshareprovider.go#L82)
{{< highlight toml >}}
[grpc.services.ocmshareprovider]
offer_webapp = false
{{< /highlight >}}
{{% /dir %}}

{{% dir name="webapp_name" type="string" default="" %}}
Name advertised for the webapp protocol when offer_webapp is true. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/ocmshareprovider/ocmshareprovider.go#L83)
{{< highlight toml >}}
[grpc.services.ocmshareprovider]
webapp_name = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="webapp_endpoint" type="string" default="" %}}
Absolute URL of the webapp opener when offer_webapp is true. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/ocmshareprovider/ocmshareprovider.go#L84)
{{< highlight toml >}}
[grpc.services.ocmshareprovider]
webapp_endpoint = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="signing_key" type="string" default="" %}}
Path to the PEM file of the Ed25519 private key that signs the shares that this server sends. Use the key of the wellknown ocmprovider. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/ocmshareprovider/ocmshareprovider.go#L85)
{{< highlight toml >}}
[grpc.services.ocmshareprovider]
signing_key = ""
{{< /highlight >}}
{{% /dir %}}

