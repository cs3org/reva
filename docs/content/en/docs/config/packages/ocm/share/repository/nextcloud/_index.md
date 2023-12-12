---
title: "nextcloud"
linkTitle: "nextcloud"
weight: 10
description: >
  Configuration for the nextcloud service
---

# _struct: ShareManagerConfig_

{{% dir name="endpoint" type="string" default="" %}}
The Nextcloud backend endpoint for user check [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/ocm/share/repository/nextcloud/nextcloud.go#L60)
{{< highlight toml >}}
[ocm.share.repository.nextcloud]
endpoint = ""
{{< /highlight >}}
{{% /dir %}}

