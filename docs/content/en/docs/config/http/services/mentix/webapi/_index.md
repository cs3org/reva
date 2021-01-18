---
title: "webapi"
linkTitle: "webapi"
weight: 10
description: >
    Configuration for the WebAPI of the Mentix service
---

{{% pageinfo %}}
The WebAPI of Mentix supports both importing and exporting of mesh data via an HTTP endpoint. Both the im- and exporter are configured separately.
{{% /pageinfo %}}

## Importer

The WebAPI importer receives a single _plain_ Mentix site through an HTTP `POST` request; service types are currently not supported.

The importer supports two actions that must be passed in the URL:
```
https://sciencemesh.example.com/mentix/webapi/?action=<value>
```
Currently, the following actions are supported:
- `register`: Registers a new site
- `unregister`: Unregisters an existing site

For all actions, the site data must be sent as JSON data. If the call succeeded, status 200 is returned.

{{% dir name="endpoint" type="string" default="/sites" %}}
The endpoint where the mesh data can be sent to.
{{< highlight toml >}}
[http.services.mentix.importers.webapi]
endpoint = "/data"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="is_protected" type="bool" default="false" %}}
Whether the endpoint requires authentication.
{{< highlight toml >}}
[http.services.mentix.importers.webapi]
is_protected = true
{{< /highlight >}}
{{% /dir %}}

{{% dir name="enabled_connectors" type="[]string" default="" %}}
A list of all enabled connectors for the importer. Must always be provided.
{{< highlight toml >}}
[http.services.mentix.importers.webapi]
enabled_connectors = ["localfile"]
{{< /highlight >}}
{{% /dir %}}

## Exporter

The WebAPI exporter exposes the _plain_ Mentix data via an HTTP endpoint.

{{% dir name="endpoint" type="string" default="/sites" %}}
The endpoint where the mesh data can be queried.
{{< highlight toml >}}
[http.services.mentix.exporters.webapi]
endpoint = "/data"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="is_protected" type="bool" default="false" %}}
Whether the endpoint requires authentication.
{{< highlight toml >}}
[http.services.mentix.exporters.webapi]
is_protected = true
{{< /highlight >}}
{{% /dir %}}

{{% dir name="enabled_connectors" type="[]string" default="*" %}}
A list of all enabled connectors for the exporter.
{{< highlight toml >}}
[http.services.mentix.exporters.webapi]
enabled_connectors = ["gocdb"]
{{< /highlight >}}
{{% /dir %}}
