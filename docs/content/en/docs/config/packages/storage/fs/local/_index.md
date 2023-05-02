---
title: "local"
linkTitle: "local"
weight: 10
description: >
  Configuration for the local service
---

# _struct: config_

{{% dir name="root" type="string" default="/var/tmp/reva/" %}}
Path of root directory for user storage. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/local/local.go#L35)
{{< highlight toml >}}
[storage.fs.local]
root = "/var/tmp/reva/"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="share_folder" type="string" default="/MyShares" %}}
Path for storing share references. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/local/local.go#L36)
{{< highlight toml >}}
[storage.fs.local]
share_folder = "/MyShares"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="user_layout" type="string" default="{{.Username}}" %}}
Template used for building the user's root path. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/local/local.go#L37)
{{< highlight toml >}}
[storage.fs.local]
user_layout = "{{.Username}}"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="disable_home" type="bool" default=false %}}
Enable/disable special /home handling. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/local/local.go#L38)
{{< highlight toml >}}
[storage.fs.local]
disable_home = false
{{< /highlight >}}
{{% /dir %}}

