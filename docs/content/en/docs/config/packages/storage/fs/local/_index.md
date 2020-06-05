---
title: "Local Storage"
linkTitle: "Local Storage"
weight: 10
description: >
  Configuration reference for Local Storage
---

struct config

{{% dir name="root" type="string" default="/var/tmp/reva/" %}}
Path of root directory for user storage.
{{< highlight toml >}}
[storage.fs.local]
root = "/var/tmp/reva/"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="enable_home" type="bool" default="false" %}}
Whether to have individual home directories for users.
{{< highlight toml >}}
[storage.fs.local]
enable_home = "false"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="user_layout" type="string" default="{{.Username}}" %}}
Template for user home directories
{{< highlight toml >}}
[storage.fs.local]
user_layout = "{{.Username}}"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="share_folder" type="string" default="/MyShares" %}}
Path for storing share references.
{{< highlight toml >}}
[storage.fs.local]
share_folder = "/MyShares"
{{< /highlight >}}
{{% /dir %}}

