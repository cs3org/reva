---
title: "dynamic"
linkTitle: "dynamic"
weight: 10
description: >
  Configuration for the dynamic service
---

## Configuration

{{% dir name="rules" type="map[string]string" default=nil %}}
A map from mountID to provider address [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/registry/dynamic/dynamic.go#L56)
{{< highlight toml >}}
[storage.registry.dynamic]
rules = nil
{{< /highlight >}}
{{% /dir %}}

{{% dir name="rewrites" type="map[string]string" default=nil %}}
A map from a path to an template alias to use when resolving [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/registry/dynamic/dynamic.go#L57)
{{< highlight toml >}}
[storage.registry.dynamic]
rewrites = nil
{{< /highlight >}}
{{% /dir %}}

{{% dir name="aliases" type="map[string]string" default=nil %}}
A map containing storageID aliases, can contain simple brackets [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/registry/dynamic/dynamic.go#L58)
{{< highlight toml >}}
[storage.registry.dynamic]
aliases = nil
{{< /highlight >}}
{{% /dir %}}

