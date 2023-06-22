---
title: "ocdav"
linkTitle: "ocdav"
weight: 10
description: >
  Configuration for the OCDav service
---

# _struct: Config_

{{% dir name="insecure" type="bool" default=false %}}
Whether to skip certificate checks when sending requests. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/owncloud/ocdav/ocdav.go#L104)
{{< highlight toml >}}
[http.services.owncloud.ocdav]
insecure = false
{{< /highlight >}}
{{% /dir %}}

{{% dir name="notifications" type="map[string]interface{}" default=Settingsg for the Notification Helper %}}
 [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/owncloud/ocdav/ocdav.go#L115)
{{< highlight toml >}}
[http.services.owncloud.ocdav]
notifications = Settingsg for the Notification Helper
{{< /highlight >}}
{{% /dir %}}

