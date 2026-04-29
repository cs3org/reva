---
title: "oidc"
linkTitle: "oidc"
weight: 10
description: >
  Configuration for the oidc service
---

# _struct: config_

{{% dir name="insecure" type="bool" default=false %}}
Whether to skip certificate checks when sending requests. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidc/oidc.go#L65)
{{< highlight toml >}}
[auth.manager.oidc]
insecure = false
{{< /highlight >}}
{{% /dir %}}

{{% dir name="issuer" type="string" default="" %}}
The issuer of the OIDC token. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidc/oidc.go#L66)
{{< highlight toml >}}
[auth.manager.oidc]
issuer = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="default_id_claim" type="string" default="sub" %}}
The default claim used as the user ID when no per-IDP override is configured. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidc/oidc.go#L67)
{{< highlight toml >}}
[auth.manager.oidc]
default_id_claim = "sub"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="idp_to_id_claim" type="map[string]string" default= %}}
Per-IDP claim to use as the user ID, keyed by the value of the identity_provider token claim. Overrides default_id_claim for matching IDPs. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidc/oidc.go#L68)
{{< highlight toml >}}
[auth.manager.oidc]
idp_to_id_claim = 
{{< /highlight >}}
{{% /dir %}}

{{% dir name="uid_claim" type="string" default="" %}}
The claim containing the UID of the user. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidc/oidc.go#L69)
{{< highlight toml >}}
[auth.manager.oidc]
uid_claim = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="gid_claim" type="string" default="" %}}
The claim containing the GID of the user. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidc/oidc.go#L70)
{{< highlight toml >}}
[auth.manager.oidc]
gid_claim = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="gatewaysvc" type="string" default="" %}}
The endpoint at which the GRPC gateway is exposed. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidc/oidc.go#L71)
{{< highlight toml >}}
[auth.manager.oidc]
gatewaysvc = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="users_mapping" type="string" default="" %}}
 The optional OIDC users mapping file path [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidc/oidc.go#L72)
{{< highlight toml >}}
[auth.manager.oidc]
users_mapping = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="group_claim" type="string" default="" %}}
 The group claim to be looked up to map the user (default to 'groups'). [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidc/oidc.go#L73)
{{< highlight toml >}}
[auth.manager.oidc]
group_claim = ""
{{< /highlight >}}
{{% /dir %}}

