---
title: "oidc"
linkTitle: "oidc"
weight: 10
description: >
  Configuration for the oidc service
---

# _struct: config_

{{% dir name="insecure" type="bool" default=false %}}
Whether to skip certificate checks when sending requests. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidc/oidc.go#L53)
{{< highlight toml >}}
[auth.manager.oidc]
insecure = false
{{< /highlight >}}
{{% /dir %}}

{{% dir name="issuer" type="string" default="" %}}
The issuer of the OIDC token. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidc/oidc.go#L54)
{{< highlight toml >}}
[auth.manager.oidc]
issuer = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="id_claim" type="string" default="sub" %}}
The claim containing the ID of the user. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidc/oidc.go#L55)
{{< highlight toml >}}
[auth.manager.oidc]
id_claim = "sub"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="uid_claim" type="string" default="" %}}
The claim containing the UID of the user. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidc/oidc.go#L56)
{{< highlight toml >}}
[auth.manager.oidc]
uid_claim = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="gid_claim" type="string" default="" %}}
The claim containing the GID of the user. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidc/oidc.go#L57)
{{< highlight toml >}}
[auth.manager.oidc]
gid_claim = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="gatewaysvc" type="string" default="" %}}
The endpoint at which the GRPC gateway is exposed. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidc/oidc.go#L58)
{{< highlight toml >}}
[auth.manager.oidc]
gatewaysvc = ""
{{< /highlight >}}
{{% /dir %}}

