---
title: "rest"
linkTitle: "rest"
weight: 10
description: >
  Configuration for the rest service
---

# _struct: config_

{{% dir name="redis_address" type="string" default="localhost:6379" %}}
The address at which the redis server is running [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/cbox/group/rest/rest.go#L55)
{{< highlight toml >}}
[cbox.group.rest]
redis_address = "localhost:6379"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="group_members_cache_expiration" type="int" default=5 %}}
The time in minutes for which the members of a group would be cached [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/cbox/group/rest/rest.go#L61)
{{< highlight toml >}}
[cbox.group.rest]
group_members_cache_expiration = 5
{{< /highlight >}}
{{% /dir %}}

{{% dir name="id_provider" type="string" default="http://cernbox.cern.ch" %}}
The OIDC Provider [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/cbox/group/rest/rest.go#L63)
{{< highlight toml >}}
[cbox.group.rest]
id_provider = "http://cernbox.cern.ch"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="api_base_url" type="string" default="https://authorization-service-api-dev.web.cern.ch/api/v1.0" %}}
Base API Endpoint [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/cbox/group/rest/rest.go#L65)
{{< highlight toml >}}
[cbox.group.rest]
api_base_url = "https://authorization-service-api-dev.web.cern.ch/api/v1.0"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="client_id" type="string" default="-" %}}
Client ID needed to authenticate [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/cbox/group/rest/rest.go#L67)
{{< highlight toml >}}
[cbox.group.rest]
client_id = "-"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="client_secret" type="string" default="-" %}}
Client Secret [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/cbox/group/rest/rest.go#L69)
{{< highlight toml >}}
[cbox.group.rest]
client_secret = "-"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="oidc_token_endpoint" type="string" default="https://keycloak-dev.cern.ch/auth/realms/cern/api-access/token" %}}
Endpoint to generate token to access the API [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/cbox/group/rest/rest.go#L72)
{{< highlight toml >}}
[cbox.group.rest]
oidc_token_endpoint = "https://keycloak-dev.cern.ch/auth/realms/cern/api-access/token"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="target_api" type="string" default="authorization-service-api" %}}
The target application for which token needs to be generated [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/cbox/group/rest/rest.go#L74)
{{< highlight toml >}}
[cbox.group.rest]
target_api = "authorization-service-api"
{{< /highlight >}}
{{% /dir %}}

