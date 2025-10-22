---
title: "permissions"
linkTitle: "permissions"
weight: 10
description: >
  Configuration for the permissions service
---

# _struct: config_

{{% dir name="driver" type="string" default="localhome" %}}
The permission driver to be used. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/permissions/permissions.go#L46)
{{< highlight toml >}}
[grpc.services.permissions]
driver = "localhome"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="drivers" type="map[string]map[string]interface{}" default="permission" %}}
 [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/permissions/permissions.go#L47)
{{< highlight toml >}}
[grpc.services.permissions.drivers.permission]

{{< /highlight >}}
{{% /dir %}}

