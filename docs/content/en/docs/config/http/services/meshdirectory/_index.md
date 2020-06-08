---
title: "meshdirectory"
linkTitle: "meshdirectory"
weight: 10
description: >
  Configuration for the Mesh Directory service
---

{{% dir name="static" type="string" default="static" %}}
Path to a static directory containing a UI frontend for the service.
This directory must contain an **index.html** file at least.
{{< highlight toml >}}
[http.services.meshdirectory]
static = "/path/to/static"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="driver" type="string" default="json" %}}
Which driver to use as a source of Mesh Provider data. Currently supported drivers are: **mentix**, **json**.
{{< highlight toml >}}
[http.services.meshdirectory]
driver = "mentix"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="url" type="string" default="http://localhost:9600/" %}}
URL of Mentix HTTP service when using the Mentix driver to query for Mesh Providers.
{{< highlight toml >}}
[http.services.meshdirectory.drivers.mentix]
url = "http://localhost:9600/"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="providers" type="string" default="" %}}
Source file containing Mesh Provider data when using the JSON driver.
{{< highlight toml >}}
[http.services.meshdirectory.drivers.json]
providers = "providers.json"
{{< /highlight >}}
{{% /dir %}}

The expected format of the JSON source file is:
{{< highlight json >}}
{
  "providers": [
    {
      "Name": "cernbox",
      "FullName": "CERNBox",
      "Organization": "CERN",
      "Domain": "cernbox.cern.ch",
      "Homepage": "https://cernbox.cern.ch",
      "Description": "CERNBox provides cloud data storage to all CERN users.",
      "Services": [
        {
          "Type": {
            "Name": "OCM",
            "Description": "CERNBox Open Cloud Mesh API"
          },
          "Name": "CERNBox - OCM API",
          "Path": "",
          "IsMonitored": true,
          "Properties": {},
          "Host": "",
          "AdditionalEndpoints": [
            {
              "IsMonitored": true,
              "Name": "OCM API",
              "Path": "https://cernbox.cern.ch/ocm",
              "Properties": {},
              "Type": {
                "Description": "Open Cloud Mesh",
                "Name": "OCM"
              }
            }
          ]
        }
      ]
    }
  ]
}
{{< /highlight >}}


