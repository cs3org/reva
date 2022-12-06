---
title: "mentix"
linkTitle: "mentix"
weight: 10
description: >
  Configuration for the mentix service
---

# _struct: config_

{{% dir name="insecure" type="bool" default=false %}}
Whether to skip certificate checks when sending requests. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/ocm/provider/authorizer/mentix/mentix.go#L81)
{{< highlight toml >}}
[ocm.provider.authorizer.mentix]
insecure = false
{{< /highlight >}}
{{% /dir %}}

