---
title: "rest"
linkTitle: "rest"
weight: 10
description: >
  Configuration for the rest service
---

# _struct: config_

{{% dir name="api_base_url" type="string" default="https://authorization-service-api-dev.web.cern.ch/api/v1.0" %}}
Base API Endpoint [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/user/manager/rest/rest.go#L59)
{{< highlight toml >}}
[user.manager.rest]
api_base_url = "https://authorization-service-api-dev.web.cern.ch/api/v1.0"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="client_id" type="string" default="-" %}}
Client ID needed to authenticate [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/user/manager/rest/rest.go#L62)
{{< highlight toml >}}
[user.manager.rest]
client_id = "-"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="oidc_token_endpoint" type="string" default="https://keycloak-dev.cern.ch/auth/realms/cern/api-access/token" %}}
Endpoint to generate token to access the API [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/user/manager/rest/rest.go#L66)
{{< highlight toml >}}
[user.manager.rest]
oidc_token_endpoint = "https://keycloak-dev.cern.ch/auth/realms/cern/api-access/token"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="target_api" type="string" default="authorization-service-api" %}}
The target application for which token needs to be generated [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/user/manager/rest/rest.go#L68)
{{< highlight toml >}}
[user.manager.rest]
target_api = "authorization-service-api"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="id_provider" type="string" default="http://cernbox.cern.ch" %}}
The OIDC Provider [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/user/manager/rest/rest.go#L71)
{{< highlight toml >}}
[user.manager.rest]
id_provider = "http://cernbox.cern.ch"
{{< /highlight >}}
{{% /dir %}}

