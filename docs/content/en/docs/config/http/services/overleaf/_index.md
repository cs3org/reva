---
title: "overleaf"
linkTitle: "overleaf"
weight: 10
description: >
  Configuration for the overleaf service
---

# _struct: config_

{{% dir name="app_name" type="string" default="" %}}
The App user-friendly name. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/overleaf/overleaf.go#L58)
{{< highlight toml >}}
[http.services.overleaf]
app_name = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="archiver_url" type="string" default="" %}}
Internet-facing URL of the archiver service, used to serve the files to Overleaf. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/overleaf/overleaf.go#L59)
{{< highlight toml >}}
[http.services.overleaf]
archiver_url = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="app_url" type="string" default="" %}}
The App URL. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/overleaf/overleaf.go#L60)
{{< highlight toml >}}
[http.services.overleaf]
app_url = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="insecure" type="bool" default=false %}}
Whether to skip certificate checks when sending requests. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/overleaf/overleaf.go#L61)
{{< highlight toml >}}
[http.services.overleaf]
insecure = false
{{< /highlight >}}
{{% /dir %}}

