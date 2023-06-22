---
title: "ocmshareprovider"
linkTitle: "ocmshareprovider"
weight: 10
description: >
  Configuration for the OCM Share Provider service
---

# _struct: config_

{{% dir name="provider_domain" type="string" default="The same domain registered in the provider authorizer" %}}
 [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/ocmshareprovider/ocmshareprovider.go#L64)
{{< highlight toml >}}
[grpc.services.ocmshareprovider]
provider_domain = "The same domain registered in the provider authorizer"
{{< /highlight >}}
{{% /dir %}}

