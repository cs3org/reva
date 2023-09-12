---
title: "overleaf"
linkTitle: "overleaf"
weight: 10
description: >
  Configuration for the overleaf service
---

# _struct: config_

{{% dir name="mime_types" type="[]string" default=nil %}}
Inherited from the appprovider. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/overleaf/overleaf.go#L69)
{{< highlight toml >}}
[app.provider.overleaf]
mime_types = nil
{{< /highlight >}}
{{% /dir %}}

{{% dir name="iop_secret" type="string" default="" %}}
The IOP secret used to connect to the wopiserver. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/overleaf/overleaf.go#L70)
{{< highlight toml >}}
[app.provider.overleaf]
iop_secret = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="app_name" type="string" default="" %}}
The App user-friendly name. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/overleaf/overleaf.go#L71)
{{< highlight toml >}}
[app.provider.overleaf]
app_name = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="app_icon_uri" type="string" default="" %}}
A URI to a static asset which represents the app icon. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/overleaf/overleaf.go#L72)
{{< highlight toml >}}
[app.provider.overleaf]
app_icon_uri = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="folder_base_url" type="string" default="" %}}
The base URL to generate links to navigate back to the containing folder. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/overleaf/overleaf.go#L73)
{{< highlight toml >}}
[app.provider.overleaf]
folder_base_url = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="app_url" type="string" default="" %}}
The App URL. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/overleaf/overleaf.go#L74)
{{< highlight toml >}}
[app.provider.overleaf]
app_url = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="app_int_url" type="string" default="" %}}
The internal app URL in case of dockerized deployments. Defaults to AppURL [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/overleaf/overleaf.go#L75)
{{< highlight toml >}}
[app.provider.overleaf]
app_int_url = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="app_api_key" type="string" default="" %}}
The API key used by the app, if applicable. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/overleaf/overleaf.go#L76)
{{< highlight toml >}}
[app.provider.overleaf]
app_api_key = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="jwt_secret" type="string" default="" %}}
The JWT secret to be used to retrieve the token TTL. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/overleaf/overleaf.go#L77)
{{< highlight toml >}}
[app.provider.overleaf]
jwt_secret = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="app_desktop_only" type="bool" default=false %}}
Specifies if the app can be opened only on desktop. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/overleaf/overleaf.go#L78)
{{< highlight toml >}}
[app.provider.overleaf]
app_desktop_only = false
{{< /highlight >}}
{{% /dir %}}

