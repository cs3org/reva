---
title: "usershareprovider"
linkTitle: "usershareprovider"
weight: 10
description: >
  Configuration for the User Share Provider service
---

{{% pageinfo %}}
TODO
{{% /pageinfo %}}

{{% dir name="prefix" type="string" default="oauth2" %}}
Where the HTTP service is exposed.
{{< highlight toml >}}
[grpc.services.storageprovider]
prefix = "/"
{{< /highlight >}}
{{% /dir %}}

