---
title: "ceph"
linkTitle: "ceph"
weight: 10
description: >
  Configuration for the ceph service
---

# _struct: config_

{{% dir name="root" type="string" default="/var/tmp/reva/" %}}
Path of root directory for user storage. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/ceph/ceph.go#L34)
{{< highlight toml >}}
[storage.fs.ceph]
root = "/var/tmp/reva/"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="share_folder" type="string" default="/MyShares" %}}
Path for storing share references. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/ceph/ceph.go#L35)
{{< highlight toml >}}
[storage.fs.ceph]
share_folder = "/MyShares"
{{< /highlight >}}
{{% /dir %}}

