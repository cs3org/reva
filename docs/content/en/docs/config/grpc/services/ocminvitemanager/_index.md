---
title: "ocminvitemanager"
linkTitle: "ocminvitemanager"
weight: 10
description: >
  Configuration for the ocminvitemanager service
---

# _struct: config_

{{% dir name="provider_domain" type="string" default="The same domain registered in the provider authorizer" %}}
 [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/ocminvitemanager/ocminvitemanager.go#L55)
{{< highlight toml >}}
[grpc.services.ocminvitemanager]
provider_domain = "The same domain registered in the provider authorizer"
{{< /highlight >}}
{{% /dir %}}

