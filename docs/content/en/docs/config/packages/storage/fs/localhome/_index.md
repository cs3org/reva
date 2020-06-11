---
title: "localhome"
linkTitle: "localhome"
weight: 10
description: >
  Configuration for the localhome service
---

# _struct: config_

{{% dir name="root" type="string" default="/var/tmp/reva/" %}}
Path of root directory for user storage. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/localhome/localhome.go#L34)
{{< highlight toml >}}
[storage.fs.localhome]
root = "/var/tmp/reva/"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="share_folder" type="string" default="/MyShares" %}}
Path for storing share references. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/localhome/localhome.go#L35)
{{< highlight toml >}}
[storage.fs.localhome]
share_folder = "/MyShares"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="user_layout" type="string" default="{{.Username}}" %}}
Template for user home directories [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/localhome/localhome.go#L36)
{{< highlight toml >}}
[storage.fs.localhome]
user_layout = "{{.Username}}"
{{< /highlight >}}
{{% /dir %}}

