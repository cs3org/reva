---
title: "wopi"
linkTitle: "wopi"
weight: 10
description: >
  Configuration for the wopi service
---

# _struct: config_

{{% dir name="mime_types" type="[]string" default=nil %}}
Inherited from the appprovider. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/wopi/wopi.go#L79)
{{< highlight toml >}}
[app.provider.wopi]
mime_types = nil
{{< /highlight >}}
{{% /dir %}}

{{% dir name="iop_secret" type="string" default="" %}}
The IOP secret used to connect to the wopiserver. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/wopi/wopi.go#L80)
{{< highlight toml >}}
[app.provider.wopi]
iop_secret = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="wopi_url" type="string" default="" %}}
The wopiserver's URL. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/wopi/wopi.go#L81)
{{< highlight toml >}}
[app.provider.wopi]
wopi_url = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="app_name" type="string" default="" %}}
The App user-friendly name. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/wopi/wopi.go#L82)
{{< highlight toml >}}
[app.provider.wopi]
app_name = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="app_icon_uri" type="string" default="" %}}
A URI to a static asset which represents the app icon. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/wopi/wopi.go#L83)
{{< highlight toml >}}
[app.provider.wopi]
app_icon_uri = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="folder_base_url" type="string" default="" %}}
The base URL to generate links to navigate back to the containing folder. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/wopi/wopi.go#L84)
{{< highlight toml >}}
[app.provider.wopi]
folder_base_url = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="app_url" type="string" default="" %}}
The App URL. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/wopi/wopi.go#L85)
{{< highlight toml >}}
[app.provider.wopi]
app_url = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="app_int_url" type="string" default="" %}}
The internal app URL in case of dockerized deployments. Defaults to AppURL [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/wopi/wopi.go#L86)
{{< highlight toml >}}
[app.provider.wopi]
app_int_url = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="app_api_key" type="string" default="" %}}
The API key used by the app, if applicable. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/wopi/wopi.go#L87)
{{< highlight toml >}}
[app.provider.wopi]
app_api_key = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="jwt_secret" type="string" default="" %}}
The JWT secret to be used to retrieve the token TTL. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/wopi/wopi.go#L88)
{{< highlight toml >}}
[app.provider.wopi]
jwt_secret = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="app_desktop_only" type="bool" default=false %}}
Specifies if the app can be opened only on desktop. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/wopi/wopi.go#L89)
{{< highlight toml >}}
[app.provider.wopi]
app_desktop_only = false
{{< /highlight >}}
{{% /dir %}}

