---
title: "ocminvitemanager"
linkTitle: "ocminvitemanager"
weight: 10
description: >
  Configuration for the ocminvitemanager service
---

# _struct: config_

{{% dir name="provider_domain" type="string" default="The same domain registered in the provider authorizer" %}}
 [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/ocminvitemanager/ocminvitemanager.go#L66)
{{< highlight toml >}}
[grpc.services.ocminvitemanager]
provider_domain = "The same domain registered in the provider authorizer"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="signing_key" type="string" default="" %}}
Path to the PEM file of the Ed25519 private key that signs the accepted invites sent to OCM servers that list http-sig. Use the key of the wellknown ocmprovider. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/ocminvitemanager/ocminvitemanager.go#L67)
{{< highlight toml >}}
[grpc.services.ocminvitemanager]
signing_key = ""
{{< /highlight >}}
{{% /dir %}}

