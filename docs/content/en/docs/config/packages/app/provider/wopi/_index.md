---
title: "wopi"
linkTitle: "wopi"
weight: 10
description: >
  Configuration for the wopi service
---

# _struct: config_

{{% dir name="iop_secret" type="string" default="" %}}
The IOP secret used to connect to the wopiserver. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/wopi/wopi.go#L52)
{{< highlight toml >}}
[app.provider.wopi]
iop_secret = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="wopi_url" type="string" default="" %}}
The wopiserver's URL. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/wopi/wopi.go#L53)
{{< highlight toml >}}
[app.provider.wopi]
wopi_url = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="app_name" type="string" default="" %}}
The App user-friendly name. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/wopi/wopi.go#L54)
{{< highlight toml >}}
[app.provider.wopi]
app_name = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="app_url" type="string" default="" %}}
The App URL. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/wopi/wopi.go#L55)
{{< highlight toml >}}
[app.provider.wopi]
app_url = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="app_int_url" type="string" default="" %}}
The internal app URL in case of dockerized deployments. Defaults to AppURL [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/wopi/wopi.go#L56)
{{< highlight toml >}}
[app.provider.wopi]
app_int_url = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="app_api_key" type="string" default="" %}}
The API key used by the app, if applicable. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/app/provider/wopi/wopi.go#L57)
{{< highlight toml >}}
[app.provider.wopi]
app_api_key = ""
{{< /highlight >}}
{{% /dir %}}

