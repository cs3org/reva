---
title: "oidcprovider"
linkTitle: "oidcprovider"
weight: 10
description: >
  Configuration for the OIDC Provider service
---

{{% pageinfo %}}
TODO
{{% /pageinfo %}}

{{% dir name="prefix" type="string" default="oauth2" %}}
Where the HTTP service is exposed.
{{< highlight toml >}}
[http.services.oidcprovider]
prefix = "/"
{{< /highlight >}}
{{% /dir %}}

