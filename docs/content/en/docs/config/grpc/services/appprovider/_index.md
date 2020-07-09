---
title: "appprovider"
linkTitle: "appprovider"
weight: 10
description: >
  Configuration for the appprovider service
---

# _struct: config_

{{% dir name="iopsecret" type="string" default="" %}}
The iopsecret used to connect to the wopiserver. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/appprovider/appprovider.go#L59)
{{< highlight toml >}}
[grpc.services.appprovider]
iopsecret = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="wopiurl" type="string" default="" %}}
The wopiserver's url. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/appprovider/appprovider.go#L60)
{{< highlight toml >}}
[grpc.services.appprovider]
wopiurl = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="uirul" type="string" default="" %}}
URL to application (eg collabora) URL. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/appprovider/appprovider.go#L61)
{{< highlight toml >}}
[grpc.services.appprovider]
uirul = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="storageid" type="string" default="" %}}
The storage id used by the wopiserver to look up the file or storage id defaults to default by the wopiserver if empty. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/appprovider/appprovider.go#L62)
{{< highlight toml >}}
[grpc.services.appprovider]
storageid = ""
{{< /highlight >}}
{{% /dir %}}

