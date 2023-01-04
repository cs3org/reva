---
title: "ocmshareprovider"
linkTitle: "ocmshareprovider"
weight: 10
description: >
  Configuration for the OCM Share Provider service
---

{{% pageinfo %}}
TODO
{{% /pageinfo %}}

{{% dir name="driver" type="string" default="" %}}
Driver to use. If you use a Nextcloud or ownCloud 10 backend,
you should use the "nextcloud" driver, and than provide
further configuration for it in 
grpc.services.ocmshareprovider.drivers.nextcloud.

{{< highlight toml >}}
[grpc.services.ocmshareprovider]
driver = "nextcloud"
{{< /highlight >}}
{{% /dir %}}
