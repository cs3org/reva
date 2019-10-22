---
title: "helloworld"
linkTitle: "helloworld"
weight: 10
description: >
  Configuration for the HelloWorld service
---

{{% pageinfo %}}
TODO
{{% /pageinfo %}}

{{% dir name="message" type="string" default="Hello" %}}
The hello message to return.
{{< highlight toml >}}
[grpc.services.helloworld]
message = "Ola"
{{< /highlight >}}
{{% /dir %}}

