---
title: "local"
linkTitle: "local"
weight: 10
description: >
  Configuration for the local service
---

# _struct: config_

{{% dir name="root" type="string" default="/var/tmp/reva/" %}}
Path of root directory for user storage. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/local/local.go#L52)
{{< highlight toml >}}
[storage.fs.local]
root = "/var/tmp/reva/"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="disable_home" type="bool" default=false %}}
Whether to not have individual home directories for users. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/local/local.go#L53)
{{< highlight toml >}}
[storage.fs.local]
disable_home = false
{{< /highlight >}}
{{% /dir %}}

{{% dir name="user_layout" type="string" default="{{.Username}}" %}}
Template for user home directories [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/local/local.go#L54)
{{< highlight toml >}}
[storage.fs.local]
user_layout = "{{.Username}}"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="share_folder" type="string" default="/MyShares" %}}
Path for storing share references. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/local/local.go#L55)
{{< highlight toml >}}
[storage.fs.local]
share_folder = "/MyShares"
{{< /highlight >}}
{{% /dir %}}

