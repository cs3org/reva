---
title: "oidc"
linkTitle: "oidc"
weight: 10
description: >
  Configuration for the oidc service
---

# _struct: config_

{{% dir name="insecure" type="bool" default=false %}}
<<<<<<< HEAD
Whether to skip certificate checks when sending requests. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidc/oidc.go#L55)
=======
Whether to skip certificate checks when sending requests. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidc/oidc.go#L61)
>>>>>>> master
{{< highlight toml >}}
[auth.manager.oidc]
insecure = false
{{< /highlight >}}
{{% /dir %}}

{{% dir name="issuer" type="string" default="" %}}
<<<<<<< HEAD
The issuer of the OIDC token. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidc/oidc.go#L56)
=======
The issuer of the OIDC token. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidc/oidc.go#L62)
>>>>>>> master
{{< highlight toml >}}
[auth.manager.oidc]
issuer = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="id_claim" type="string" default="sub" %}}
<<<<<<< HEAD
The claim containing the ID of the user. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidc/oidc.go#L57)
=======
The claim containing the ID of the user. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidc/oidc.go#L63)
>>>>>>> master
{{< highlight toml >}}
[auth.manager.oidc]
id_claim = "sub"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="uid_claim" type="string" default="" %}}
<<<<<<< HEAD
The claim containing the UID of the user. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidc/oidc.go#L58)
=======
The claim containing the UID of the user. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidc/oidc.go#L64)
>>>>>>> master
{{< highlight toml >}}
[auth.manager.oidc]
uid_claim = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="gid_claim" type="string" default="" %}}
<<<<<<< HEAD
The claim containing the GID of the user. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidc/oidc.go#L59)
=======
The claim containing the GID of the user. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidc/oidc.go#L65)
>>>>>>> master
{{< highlight toml >}}
[auth.manager.oidc]
gid_claim = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="gatewaysvc" type="string" default="" %}}
<<<<<<< HEAD
The endpoint at which the GRPC gateway is exposed. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidc/oidc.go#L60)
=======
The endpoint at which the GRPC gateway is exposed. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidc/oidc.go#L66)
>>>>>>> master
{{< highlight toml >}}
[auth.manager.oidc]
gatewaysvc = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="users_mapping" type="string" default="" %}}
 The optional OIDC users mapping file path [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidc/oidc.go#L67)
{{< highlight toml >}}
[auth.manager.oidc]
users_mapping = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="group_claim" type="string" default="" %}}
 The group claim to be looked up to map the user (default to 'groups'). [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/oidc/oidc.go#L68)
{{< highlight toml >}}
[auth.manager.oidc]
group_claim = ""
{{< /highlight >}}
{{% /dir %}}

