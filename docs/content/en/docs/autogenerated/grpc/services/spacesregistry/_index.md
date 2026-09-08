---
title: "spacesregistry"
linkTitle: "spacesregistry"
weight: 10
description: >
  Configuration for the spacesregistry service
---

# _struct: config_

{{% dir name="space_resolution_timeout" type="int" default=nil %}}
Timeout to resolve a space with stat: if it does not respond within the given time (defaults to 3 secs), it is skipped [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/spacesregistry/spacesregistry.go#L76)
{{< highlight toml >}}
[grpc.services.spacesregistry]
space_resolution_timeout = nil
{{< /highlight >}}
{{% /dir %}}

