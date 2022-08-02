---
title: "nextcloud"
linkTitle: "nextcloud"
weight: 10
description: >
  Configuration for the nextcloud service
---

# _struct: UserManagerConfig_

{{% dir name="endpoint" type="string" default="" %}}
The Nextcloud backend endpoint for user management [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/user/manager/nextcloud/nextcloud.go#L55)
{{< highlight toml >}}
[user.manager.nextcloud]
endpoint = ""
{{< /highlight >}}
{{% /dir %}}

