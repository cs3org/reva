---
title: "mentix"
linkTitle: "mentix"
weight: 10
description: >
    Configuration for the Mentix service
---

{{% pageinfo %}}
Mentix (_**Me**sh E**nti**ty E**x**changer_) is a service to read and write mesh topology data to and from one or more sources (e.g., a GOCDB instance) and export it to various targets like an HTTP endpoint or Prometheus.
{{% /pageinfo %}}

## General settings
{{% dir name="prefix" type="string" default="mentix" %}}
The relative root path of all exposed HTTP endpoints of Mentix.
{{< highlight toml >}}
[http.services.mentix]
prefix = "/mentix"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="update_interval" type="string" default="1h" %}}
How frequently Mentix should pull and update the mesh data. Supports common time duration strings, like "1h30m", "1d" etc.
{{< highlight toml >}}
[http.services.mentix]
update_interval = "15m"
{{< /highlight >}}
{{% /dir %}}

## Connectors
Mentix is decoupled from the actual sources of the mesh data by using so-called _connectors_. A connector is used to gather the data from a certain source, which are then converted into Mentix' own internal format.

_Supported connectors:_

- **gocdb** 
The [GOCDB](https://wiki.egi.eu/wiki/GOCDB/Documentation_Index) is a database specifically designed to organize the topology of a mesh of distributed sites and services. In order to use GOCDB with Mentix, its instance address has to be configured (see [here](gocdb)).

- **localfile**
The [localfile](localfile) connector reads sites from a local JSON file. The file must contain an array of sites adhering to the `meshdata.Site` structure.
 
## Importers
Mentix can import mesh data from various sources and write it to one or more targets through the corresponding connectors.

__Supported importers:__

- **sitereg**
Mentix can import new sites via an HTTP endpoint using the `sitereg` importer. Data can be sent to the configured relative endpoint (see [here](sitereg)).

## Exporters
Mentix exposes its gathered data by using one or more _exporters_. Such exporters can, for example, write the data to a file in a specific format, or offer the data via an HTTP endpoint.

__Supported exporters:__

- **webapi**
Mentix exposes its data via an HTTP endpoint using the `webapi` exporter. Data can be retrieved at the configured relative endpoint (see [here](webapi)). The web API currently doesn't support any parameters but will most likely be extended in the future.
- **cs3api** Similar to the WebAPI exporter, the `cs3api` exporter exposes its data via an HTTP endpoint. Data can be retrieved at the configured relative endpoint (see [here](cs3api)). The data is compliant with the CS3API `ProviderInfo` structure.
- **siteloc** The Site Locations exporter `siteloc` exposes location information of all sites to be consumed by Grafana at the configured relative endpoint (see [here](siteloc)).  
- **promsd**
[Prometheus](https://prometheus.io/) supports discovering new services it should monitor via external configuration files (hence, this is called _file-based service discovery_). Mentix can create such files using the `promsd` exporter. To use this exporter, you have to specify the target output files in the configuration (see [here](promsd)). You also have to set up the discovery service in Prometheus by adding a scrape configuration like the following example to the Prometheus configuration (for more information, visit the official [Prometheus documentation](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#file_sd_config)):
  ``` scrape_configs:
      - job_name: 'sciencemesh'
        file_sd_configs:
          - files:
             - '/usr/share/prom/sciencemesh_services.json'
  ```

## Site Accounts service
Mentix uses the Reva site accounts service to query information about site accounts. The following settings must be configured properly:

{{% dir name="url" type="string" default="" %}}
The URL of the site accounts service.
{{< highlight toml >}}
[http.services.mentix.accounts]
url = "https://example.com/accounts"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="user" type="string" default="" %}}
The user name to use for basic HTTP authentication.
{{< highlight toml >}}
[http.services.mentix.accounts]
user = "hans"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="password" type="string" default="" %}}
The user password to use for basic HTTP authentication.
{{< highlight toml >}}
[http.services.mentix.accounts]
password = "secret"
{{< /highlight >}}
{{% /dir %}}
