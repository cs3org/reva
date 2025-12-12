---
title: "ocdav"
linkTitle: "ocdav"
weight: 10
description: >
  Configuration for the ocdav service
---

# _struct: Config_

{{% dir name="insecure" type="bool" default=false %}}
Whether to skip certificate checks when sending requests. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/owncloud/ocdav/ocdav.go#L120)
{{< highlight toml >}}
[http.services.owncloud.ocdav]
insecure = false
{{< /highlight >}}
{{% /dir %}}

{{% dir name="notifications" type="map[string]any" default=nil %}}
 settings for the notification helper [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/owncloud/ocdav/ocdav.go#L133)
{{< highlight toml >}}
[http.services.owncloud.ocdav]
notifications = nil
{{< /highlight >}}
{{% /dir %}}

