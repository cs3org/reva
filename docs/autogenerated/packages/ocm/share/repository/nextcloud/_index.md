---
title: "nextcloud"
linkTitle: "nextcloud"
weight: 10
description: >
  Configuration for the nextcloud service
---

# _struct: ShareManagerConfig_

{{% dir name="endpoint" type="string" default="" %}}
The Nextcloud backend endpoint for user check [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/ocm/share/repository/nextcloud/nextcloud.go#L63)
{{< highlight toml >}}
[ocm.share.repository.nextcloud]
endpoint = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="mount_id" type="string" default="" %}}
The Reva mount id to identify the storage provider proxying the EFSS. Note that only one EFSS can be proxied by a given Reva process. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/ocm/share/repository/nextcloud/nextcloud.go#L67)
{{< highlight toml >}}
[ocm.share.repository.nextcloud]
mount_id = ""
{{< /highlight >}}
{{% /dir %}}

