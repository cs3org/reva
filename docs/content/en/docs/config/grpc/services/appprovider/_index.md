---
title: "appprovider"
linkTitle: "appprovider"
weight: 10
description: >
  Configuration for the appprovider service
---

# _struct: config_

{{% dir name="mime_types" type="[]string" default=nil %}}
A list of mime types supported by this app. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/appprovider/appprovider.go#L63)
{{< highlight toml >}}
[grpc.services.appprovider]
mime_types = nil
{{< /highlight >}}
{{% /dir %}}

{{% dir name="custom_mime_types_json" type="string" default="nil" %}}
An optional mapping file with the list of supported custom file extensions and corresponding mime types. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/grpc/services/appprovider/appprovider.go#L64)
{{< highlight toml >}}
[grpc.services.appprovider]
custom_mime_types_json = "nil"
{{< /highlight >}}
{{% /dir %}}

