---
title: "helloworld"
linkTitle: "helloworld"
weight: 10
description: >
  Configuration for the HelloWorld service
---

{{% dir name="prefix" type="string" default="helloworld" %}}
Where the HTTP service is exposed.
{{< highlight toml >}}
[http.services.helloworld]
prefix = "/"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="message" type="string" default="Hello World!" %}}
The message to render.
{{< highlight toml >}}
[http.services.helloworld]
message = "Aloha!"
{{< /highlight >}}
{{% /dir %}}
