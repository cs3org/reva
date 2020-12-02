---
title: "localfile"
linkTitle: "localfile"
weight: 10
description: >
    Configuration for the local file connector of the Mentix service
---

{{% pageinfo %}}
The local file connector reads sites from a local JSON file adhering to the `meshdata.Site` structure.
{{% /pageinfo %}}

{{% dir name="file" type="string" default="" %}}
The file path.
{{< highlight toml >}}
[http.services.mentix.connectors.localfile]
file = "/etc/reva/sites.json"
{{< /highlight >}}
{{% /dir %}}
