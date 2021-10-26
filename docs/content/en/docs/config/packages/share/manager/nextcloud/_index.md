---
title: "nextcloud"
linkTitle: "nextcloud"
weight: 10
description: >
  Configuration for the nextcloud service
---

# _struct: ShareManagerConfig_

{{% dir name="endpoint" type="string" default="" %}}
The Nextcloud backend endpoint for user check [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/share/manager/nextcloud/nextcloud.go#L58)
{{< highlight toml >}}
[share.manager.nextcloud]
endpoint = ""
{{< /highlight >}}
{{% /dir %}}

