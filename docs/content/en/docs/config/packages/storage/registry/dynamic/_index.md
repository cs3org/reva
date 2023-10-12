---
title: "dynamic"
linkTitle: "dynamic"
weight: 10
description: >
  Configuration for the dynamic service
---

# _struct: config_

{{% dir name="rules" type="map[string]string" default=nil %}}
A map from mountID to provider address [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/registry/dynamic/dynamic.go#L53)
{{< highlight toml >}}
[storage.registry.dynamic]
rules = nil
{{< /highlight >}}
{{% /dir %}}

{{% dir name="rewrites" type="map[string]string" default=nil %}}
A map from a path to an template alias to use when resolving [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/registry/dynamic/dynamic.go#L54)
{{< highlight toml >}}
[storage.registry.dynamic]
rewrites = nil
{{< /highlight >}}
{{% /dir %}}

