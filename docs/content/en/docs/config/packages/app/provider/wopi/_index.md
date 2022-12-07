---
title: "wopi"
linkTitle: "wopi"
weight: 10
description: >
  Configuration for the wopi service
---

# _struct: config_

{{% dir name="iop_secret" type="string" default="" %}}
The IOP secret used to connect to the wopiserver. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/wopi/wopi.go#L60)
{{< highlight toml >}}
[app.provider.wopi]
iop_secret = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="wopi_url" type="string" default="" %}}
The wopiserver's URL. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/wopi/wopi.go#L61)
{{< highlight toml >}}
[app.provider.wopi]
wopi_url = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="wopi_folder_url_base_url" type="string" default="" %}}
The base URL to generate links to navigate back to the containing folder. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/wopi/wopi.go#L62)
{{< highlight toml >}}
[app.provider.wopi]
wopi_folder_url_base_url = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="wopi_folder_url_path_template" type="string" default="" %}}
The template to generate the folderurl path segments. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/wopi/wopi.go#L63)
{{< highlight toml >}}
[app.provider.wopi]
wopi_folder_url_path_template = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="app_name" type="string" default="" %}}
The App user-friendly name. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/wopi/wopi.go#L64)
{{< highlight toml >}}
[app.provider.wopi]
app_name = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="app_icon_uri" type="string" default="" %}}
A URI to a static asset which represents the app icon. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/wopi/wopi.go#L65)
{{< highlight toml >}}
[app.provider.wopi]
app_icon_uri = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="app_url" type="string" default="" %}}
The App URL. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/wopi/wopi.go#L66)
{{< highlight toml >}}
[app.provider.wopi]
app_url = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="app_int_url" type="string" default="" %}}
The internal app URL in case of dockerized deployments. Defaults to AppURL [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/wopi/wopi.go#L67)
{{< highlight toml >}}
[app.provider.wopi]
app_int_url = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="app_api_key" type="string" default="" %}}
The API key used by the app, if applicable. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/wopi/wopi.go#L68)
{{< highlight toml >}}
[app.provider.wopi]
app_api_key = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="jwt_secret" type="string" default="" %}}
The JWT secret to be used to retrieve the token TTL. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/wopi/wopi.go#L69)
{{< highlight toml >}}
[app.provider.wopi]
jwt_secret = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="app_desktop_only" type="bool" default=false %}}
Specifies if the app can be opened only on desktop. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/wopi/wopi.go#L70)
{{< highlight toml >}}
[app.provider.wopi]
app_desktop_only = false
{{< /highlight >}}
{{% /dir %}}

