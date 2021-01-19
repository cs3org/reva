---
title: "adminapi"
linkTitle: "adminapi"
weight: 10
description: >
    Configuration for the AdminAPI of the Mentix service
---

{{% pageinfo %}}
The AdminAPI of Mentix is a special importer that can be used to administer certain aspects of Mentix.
{{% /pageinfo %}}

The AdminAPI importer receives instructions/queries through a `POST` request.

The importer supports one action that must be passed in the URL:
```
https://sciencemesh.example.com/mentix/admin/?action=<value>
```
Currently, the following actions are supported:
- `authorize`: Authorizes or unauthorizes a site

For all actions, the site data must be sent as JSON data. If the call succeeded, status 200 is returned.

{{% dir name="endpoint" type="string" default="/admin" %}}
The endpoint where the mesh data can be sent to.
{{< highlight toml >}}
[http.services.mentix.importers.adminapi]
endpoint = "/data"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="is_protected" type="bool" default="false" %}}
Whether the endpoint requires authentication.
{{< highlight toml >}}
[http.services.mentix.importers.adminapi]
is_protected = true
{{< /highlight >}}
{{% /dir %}}

{{% dir name="enabled_connectors" type="[]string" default="" %}}
A list of all enabled connectors for the importer. Must always be provided.
{{< highlight toml >}}
[http.services.mentix.importers.adminapi]
enabled_connectors = ["localfile"]
{{< /highlight >}}
{{% /dir %}}
