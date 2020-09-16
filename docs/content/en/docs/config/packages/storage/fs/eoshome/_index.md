---
title: "eoshome"
linkTitle: "eoshome"
weight: 10
description: >
  Configuration for the eoshome service
---

# _struct: config_

{{% dir name="namespace" type="string" default="/" %}}
Namespace for metadata operations [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eoshome/eoshome.go#L38)
{{< highlight toml >}}
[storage.fs.eoshome]
namespace = "/"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="shadow_namespace" type="string" default="/.shadow" %}}
ShadowNamespace for storing shadow data [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eoshome/eoshome.go#L41)
{{< highlight toml >}}
[storage.fs.eoshome]
shadow_namespace = "/.shadow"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="uploads_namespace" type="string" default="/.uploads" %}}
UploadsNamespace for storing upload data [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eoshome/eoshome.go#L44)
{{< highlight toml >}}
[storage.fs.eoshome]
uploads_namespace = "/.uploads"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="share_folder" type="string" default="/MyShares" %}}
ShareFolder defines the name of the folder in the shadowed namespace. Ex: /eos/user/.shadow/h/hugo/MyShares [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eoshome/eoshome.go#L48)
{{< highlight toml >}}
[storage.fs.eoshome]
share_folder = "/MyShares"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="eos_binary" type="string" default="/usr/bin/eos" %}}
Location of the eos binary. Default is /usr/bin/eos. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eoshome/eoshome.go#L52)
{{< highlight toml >}}
[storage.fs.eoshome]
eos_binary = "/usr/bin/eos"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="xrdcopy_binary" type="string" default="/usr/bin/xrdcopy" %}}
Location of the xrdcopy binary. Default is /usr/bin/xrdcopy. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eoshome/eoshome.go#L56)
{{< highlight toml >}}
[storage.fs.eoshome]
xrdcopy_binary = "/usr/bin/xrdcopy"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="master_url" type="string" default="root://eos-example.org" %}}
URL of the Master EOS MGM. Default is root:eos-example.org [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eoshome/eoshome.go#L60)
{{< highlight toml >}}
[storage.fs.eoshome]
master_url = "root://eos-example.org"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="slave_url" type="string" default="root://eos-example.org" %}}
URL of the Slave EOS MGM. Default is root:eos-example.org [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eoshome/eoshome.go#L64)
{{< highlight toml >}}
[storage.fs.eoshome]
slave_url = "root://eos-example.org"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="cache_directory" type="string" default="/var/tmp/" %}}
Location on the local fs where to store reads. Defaults to os.TempDir() [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eoshome/eoshome.go#L68)
{{< highlight toml >}}
[storage.fs.eoshome]
cache_directory = "/var/tmp/"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="sec_protocol" type="string" default="-" %}}
SecProtocol specifies the xrootd security protocol to use between the server and EOS. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eoshome/eoshome.go#L71)
{{< highlight toml >}}
[storage.fs.eoshome]
sec_protocol = "-"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="keytab" type="string" default="-" %}}
Keytab specifies the location of the keytab to use to authenticate to EOS. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eoshome/eoshome.go#L74)
{{< highlight toml >}}
[storage.fs.eoshome]
keytab = "-"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="single_username" type="string" default="-" %}}
SingleUsername is the username to use when SingleUserMode is enabled [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eoshome/eoshome.go#L77)
{{< highlight toml >}}
[storage.fs.eoshome]
single_username = "-"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="user_layout" type="string" default="{{.Username}}" %}}
UserLayout wraps the internal path with user information. Example: if conf.Namespace is /eos/user and received path is /docs and the UserLayout is {{.Username}} the internal path will be: /eos/user/<username>/docs [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eoshome/eoshome.go#L83)
{{< highlight toml >}}
[storage.fs.eoshome]
user_layout = "{{.Username}}"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="enable_logging" type="bool" default=false %}}
Enables logging of the commands executed Defaults to false [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eoshome/eoshome.go#L87)
{{< highlight toml >}}
[storage.fs.eoshome]
enable_logging = false
{{< /highlight >}}
{{% /dir %}}

{{% dir name="show_hidden_sys_files" type="bool" default=false %}}
ShowHiddenSysFiles shows internal EOS files like .sys.v# and .sys.a# files. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eoshome/eoshome.go#L91)
{{< highlight toml >}}
[storage.fs.eoshome]
show_hidden_sys_files = false
{{< /highlight >}}
{{% /dir %}}

{{% dir name="force_single_user_mode" type="bool" default=false %}}
ForceSingleUserMode will force connections to EOS to use SingleUsername [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eoshome/eoshome.go#L94)
{{< highlight toml >}}
[storage.fs.eoshome]
force_single_user_mode = false
{{< /highlight >}}
{{% /dir %}}

{{% dir name="use_keytab" type="bool" default=false %}}
UseKeyTabAuth changes will authenticate requests by using an EOS keytab. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eoshome/eoshome.go#L97)
{{< highlight toml >}}
[storage.fs.eoshome]
use_keytab = false
{{< /highlight >}}
{{% /dir %}}

{{% dir name="version_invariant" type="bool" default=true %}}
Whether to maintain the same inode across various versions of a file. Requires extra metadata operations if set to true [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eoshome/eoshome.go#L101)
{{< highlight toml >}}
[storage.fs.eoshome]
version_invariant = true
{{< /highlight >}}
{{% /dir %}}

{{% dir name="gatewaysvc" type="string" default="0.0.0.0:19000" %}}
GatewaySvc stores the endpoint at which the GRPC gateway is exposed. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eoshome/eoshome.go#L104)
{{< highlight toml >}}
[storage.fs.eoshome]
gatewaysvc = "0.0.0.0:19000"
{{< /highlight >}}
{{% /dir %}}

