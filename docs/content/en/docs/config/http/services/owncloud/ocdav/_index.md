---
title: "ocdav"
linkTitle: "ocdav"
weight: 10
description: >
  Configuration for the OCDav service
---

# _struct: Config_

{{% dir name="drivers" type="map[string]map[string]interface{}" default="localhome" %}}
 [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/owncloud/ocdav/ocdav.go#L110)
{{< highlight toml >}}
[http.services.owncloud.ocdav.drivers.localhome]
root = "/var/tmp/reva/"
share_folder = "/MyShares"
user_layout = "{{.Username}}"

{{< /highlight >}}
{{% /dir %}}

