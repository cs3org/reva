---
title: "rest"
linkTitle: "rest"
weight: 10
description: >
  Configuration for the rest service
---

# _struct: config_

{{% dir name="redis_address" type="string" default="localhost:6379" %}}
The address at which the redis server is running [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/cbox/user/rest/rest.go#L53)
{{< highlight toml >}}
[cbox.user.rest]
redis_address = "localhost:6379"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="user_groups_cache_expiration" type="int" default=5 %}}
The time in minutes for which the groups to which a user belongs would be cached [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/cbox/user/rest/rest.go#L59)
{{< highlight toml >}}
[cbox.user.rest]
user_groups_cache_expiration = 5
{{< /highlight >}}
{{% /dir %}}

{{% dir name="id_provider" type="string" default="http://cernbox.cern.ch" %}}
The OIDC Provider [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/cbox/user/rest/rest.go#L61)
{{< highlight toml >}}
[cbox.user.rest]
id_provider = "http://cernbox.cern.ch"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="api_base_url" type="string" default="https://authorization-service-api-dev.web.cern.ch" %}}
Base API Endpoint [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/cbox/user/rest/rest.go#L63)
{{< highlight toml >}}
[cbox.user.rest]
api_base_url = "https://authorization-service-api-dev.web.cern.ch"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="client_id" type="string" default="-" %}}
Client ID needed to authenticate [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/cbox/user/rest/rest.go#L65)
{{< highlight toml >}}
[cbox.user.rest]
client_id = "-"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="client_secret" type="string" default="-" %}}
Client Secret [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/cbox/user/rest/rest.go#L67)
{{< highlight toml >}}
[cbox.user.rest]
client_secret = "-"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="oidc_token_endpoint" type="string" default="https://keycloak-dev.cern.ch/auth/realms/cern/api-access/token" %}}
Endpoint to generate token to access the API [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/cbox/user/rest/rest.go#L70)
{{< highlight toml >}}
[cbox.user.rest]
oidc_token_endpoint = "https://keycloak-dev.cern.ch/auth/realms/cern/api-access/token"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="target_api" type="string" default="authorization-service-api" %}}
The target application for which token needs to be generated [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/cbox/user/rest/rest.go#L72)
{{< highlight toml >}}
[cbox.user.rest]
target_api = "authorization-service-api"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="user_fetch_interval" type="int" default=3600 %}}
The time in seconds between bulk fetch of user accounts [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/cbox/user/rest/rest.go#L74)
{{< highlight toml >}}
[cbox.user.rest]
user_fetch_interval = 3600
{{< /highlight >}}
{{% /dir %}}

