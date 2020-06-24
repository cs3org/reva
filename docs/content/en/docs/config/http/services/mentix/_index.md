---
title: "mentix"
linkTitle: "mentix"
weight: 10
description: >
    Configuration for the Mentix service
---

{{% pageinfo %}}
Mentix (_**Me**sh E**nti**ty E**x**porter_) is a service to read mesh topology data from a source (e.g., a GOCDB instance) and export it to various targets like an HTTP endpoint or Prometheus.
{{% /pageinfo %}}

{{% dir name="prefix" type="string" default="mentix" %}}
The relative root path of all exposed HTTP endpoints of Mentix.
{{< highlight toml >}}
[http.services.mentix]
prefix = "/mentix"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="connector" type="string" default="gocdb" %}}
Mentix is decoupled from the actual source of the mesh data by using a so-called _connector_. A connector is used to gather the data from a certain source, which are then converted into Mentix' own internal format.

Supported values are:

- **gocdb** 
The [GOCDB](https://wiki.egi.eu/wiki/GOCDB/Documentation_Index) is a database specifically designed to organize the topology of a mesh of distributed sites and services. In order to use GOCDB with Mentix, its instance address has to be configured (see [here](gocdb)).    

{{< highlight toml >}}
[http.services.mentix]
connector = "gocdb"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="exporters" type="[]string" default="[webapi,prom_filesd]" %}}
Mentix exposes its gathered data by using one or more _exporters_. Such exporters can, for example, write the data to a file in a specific format, or offer the data via an HTTP endpoint.

Supported values are:

- **webapi**
Mentix exposes its data via an HTTP endpoint using the `webapi` exporter. Data can be retrieved at the configured relative endpoint (see [here](webapi)). The web API currently doesn't support any parameters but will most likely be extended in the future.
- **prom_filesd**
[Prometheus](https://prometheus.io/) supports discovering new services it should monitor via external configuration files (hence, this is called _file-based service discovery_). Mentix can create such files using the `prom_filesd` exporter. To use this exporter, you have to specify the target output file in the configuration (see [here](prom_filesd)). You also have to set up the discovery service in Prometheus by adding a scrape configuration like the following example to the Prometheus configuration (for more information, visit the official [Prometheus documentation](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#file_sd_config)):
  ``` scrape_configs:
      - job_name: 'sciencemesh'
        file_sd_configs:
          - files:
             - '/usr/share/prom/sciencemesh_services.json'
  ```

{{< highlight toml >}}
[http.services.mentix]
exporters = ["webapi", "prom_filesd"]
{{< /highlight >}}
{{% /dir %}}

{{% dir name="update_interval" type="string" default="1h" %}}
How frequently Mentix should pull and update the mesh data. Supports common time duration strings, like "1h30m", "1d" etc.
{{< highlight toml >}}
[http.services.mentix]
update_interval = "15m"
{{< /highlight >}}
{{% /dir %}}
