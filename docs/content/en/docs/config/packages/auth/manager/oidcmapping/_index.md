---
title: "oidcmapping"
linkTitle: "oidcmapping"
weight: 10
description: >
  Configuration for the oidcmapping service
---

# _struct: config_

{{% dir name="insecure" type="bool" default=false %}}
Whether to skip certificate checks when sending requests. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidcmapping/oidcmapping.go#L59)
{{< highlight toml >}}
[auth.manager.oidcmapping]
insecure = false
{{< /highlight >}}
{{% /dir %}}

{{% dir name="issuer" type="string" default="" %}}
The issuer of the OIDC token. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidcmapping/oidcmapping.go#L60)
{{< highlight toml >}}
[auth.manager.oidcmapping]
issuer = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="id_claim" type="string" default="sub" %}}
The claim containing the ID of the user. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidcmapping/oidcmapping.go#L61)
{{< highlight toml >}}
[auth.manager.oidcmapping]
id_claim = "sub"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="uid_claim" type="string" default="" %}}
The claim containing the UID of the user. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidcmapping/oidcmapping.go#L62)
{{< highlight toml >}}
[auth.manager.oidcmapping]
uid_claim = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="gid_claim" type="string" default="" %}}
The claim containing the GID of the user. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidcmapping/oidcmapping.go#L63)
{{< highlight toml >}}
[auth.manager.oidcmapping]
gid_claim = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="gatewaysvc" type="string" default="" %}}
The endpoint at which the GRPC gateway is exposed. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidcmapping/oidcmapping.go#L64)
{{< highlight toml >}}
[auth.manager.oidcmapping]
gatewaysvc = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="users_mapping" type="string" default="" %}}
 The optional OIDC users mapping file path [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidcmapping/oidcmapping.go#L65)
{{< highlight toml >}}
[auth.manager.oidcmapping]
users_mapping = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="group_claim" type="string" default="" %}}
 The group claim to be looked up to map the user (default to 'groups'). [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidcmapping/oidcmapping.go#L66)
{{< highlight toml >}}
[auth.manager.oidcmapping]
group_claim = ""
{{< /highlight >}}
{{% /dir %}}

