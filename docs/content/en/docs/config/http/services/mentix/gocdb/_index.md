---
title: "gocdb"
linkTitle: "gocdb"
weight: 10
description: >
    Configuration for the GOCDB connector of the Mentix service
---

{{% pageinfo %}}
When using the [GOCDB](https://wiki.egi.eu/wiki/GOCDB/Documentation_Index) connector, at least its address has to be configured.
{{% /pageinfo %}}

{{% dir name="address" type="string" default="" %}}
The address of the GOCDB instance; must be a valid URL (e.g., http://gocdb.uni-muenster.de). **Note:** The public API must be reachable under `<address>/gocdbpi/public`.
{{< highlight toml >}}
[http.services.mentix.connectors.gocdb]
address = "http://gocdb.example.com"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="scope" type="string" default="SM" %}}
The scope to use for filtering sites and services.
{{< highlight toml >}}
[http.services.mentix.connectors.gocdb]
scope = "SM"
{{< /highlight >}}
{{% /dir %}}
