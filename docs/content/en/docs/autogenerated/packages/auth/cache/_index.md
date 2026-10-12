---
title: "cache"
linkTitle: "cache"
weight: 10
description: >
  Configuration for the cache service
---

# _struct: Config_

{{% dir name="cache_size" type="int" default=50000 %}}
Maximum number of authentication outcomes kept in memory. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/cache/cache.go#L46)
{{< highlight toml >}}
[auth.cache]
cache_size = 50000
{{< /highlight >}}
{{% /dir %}}

{{% dir name="cache_ttl" type="int" default=300 %}}
Seconds an accepted credential stays cached. An entry always expires at min(TTL, expiration of the credential). A negative value disables the cache. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/cache/cache.go#L47)
{{< highlight toml >}}
[auth.cache]
cache_ttl = 300
{{< /highlight >}}
{{% /dir %}}

{{% dir name="negative_cache_ttl" type="int" default=30 %}}
Seconds a rejected credential stays cached. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/cache/cache.go#L48)
{{< highlight toml >}}
[auth.cache]
negative_cache_ttl = 30
{{< /highlight >}}
{{% /dir %}}

