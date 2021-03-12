---
title: "sitereg"
linkTitle: "sitereg"
weight: 10
description: >
    Configuration for site registration service
---

{{% pageinfo %}}
The site registration service is used to register new and unregister existing sites.
{{% /pageinfo %}}

The site registration service is used to register new and unregister existing sites.

{{% dir name="endpoint" type="string" default="/sitereg" %}}
The endpoint of the service.
{{< highlight toml >}}
[http.services.mentix.importers.sitereg]
endpoint = "/reg"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="enabled_connectors" type="[]string" default="" %}}
A list of all enabled connectors for the importer.
{{< highlight toml >}}
[http.services.mentix.importers.sitereg]
enabled_connectors = ["localfile"]
{{< /highlight >}}
{{% /dir %}}

{{% dir name="ignore_sm_sites" type="bool" default="false" %}}
If set to true, registrations from ScienceMesh sites will be ignored.
{{< highlight toml >}}
[http.services.mentix.importers.sitereg]
ignore_sm_sites = true
{{< /highlight >}}
{{% /dir %}}
