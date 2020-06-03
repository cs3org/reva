---
title: "Interceptors"
linkTitle: "Interceptors"
weight: 10
description: >
  Configuration reference for GRPC Interceptors
---

To configure an GRPC interceptor you need to follow this convention in the config file:

{{< highlight toml >}}
[grpc.interceptors.interceptor_name]
... config ...

