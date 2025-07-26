---
title: "nextcloud"
linkTitle: "nextcloud"
weight: 10
description: >
  Configuration for the nextcloud service
---

# _struct: AuthManagerConfig_

{{% dir name="endpoint" type="string" default="" %}}
The Nextcloud backend endpoint for user check [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/auth/manager/nextcloud/nextcloud.go#L55)
{{< highlight toml >}}
[auth.manager.nextcloud]
endpoint = ""
{{< /highlight >}}
{{% /dir %}}

