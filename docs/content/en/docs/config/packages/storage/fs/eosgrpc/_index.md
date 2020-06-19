---
title: "eosgrpc"
linkTitle: "eosgrpc"
weight: 10
description: >
  Configuration for the eosgrpc service
---

# _struct: config_

{{% dir name="namespace" type="string" default="/" %}}
Namespace for metadata operations [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eosgrpc/eosgrpc.go#L78)
{{< highlight toml >}}
[storage.fs.eosgrpc]
namespace = "/"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="shadow_namespace" type="string" default="/.shadow" %}}
ShadowNamespace for storing shadow data [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eosgrpc/eosgrpc.go#L81)
{{< highlight toml >}}
[storage.fs.eosgrpc]
shadow_namespace = "/.shadow"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="share_folder" type="string" default="/MyShares" %}}
ShareFolder defines the name of the folder in the shadowed namespace. Ex: /eos/user/.shadow/h/hugo/MyShares [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eosgrpc/eosgrpc.go#L85)
{{< highlight toml >}}
[storage.fs.eosgrpc]
share_folder = "/MyShares"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="eos_binary" type="string" default="/usr/bin/eos" %}}
Location of the eos binary. Default is /usr/bin/eos. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eosgrpc/eosgrpc.go#L89)
{{< highlight toml >}}
[storage.fs.eosgrpc]
eos_binary = "/usr/bin/eos"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="xrdcopy_binary" type="string" default="/usr/bin/xrdcopy" %}}
Location of the xrdcopy binary. Default is /usr/bin/xrdcopy. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eosgrpc/eosgrpc.go#L93)
{{< highlight toml >}}
[storage.fs.eosgrpc]
xrdcopy_binary = "/usr/bin/xrdcopy"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="master_url" type="string" default="root://eos-example.org" %}}
URL of the Master EOS MGM. Default is root:eos-example.org [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eosgrpc/eosgrpc.go#L97)
{{< highlight toml >}}
[storage.fs.eosgrpc]
master_url = "root://eos-example.org"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="master_grpc_uri" type="string" default="root://eos-grpc-example.org" %}}
URI of the EOS MGM grpc server Default is empty [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eosgrpc/eosgrpc.go#L101)
{{< highlight toml >}}
[storage.fs.eosgrpc]
master_grpc_uri = "root://eos-grpc-example.org"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="slave_url" type="string" default="root://eos-example.org" %}}
URL of the Slave EOS MGM. Default is root:eos-example.org [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eosgrpc/eosgrpc.go#L105)
{{< highlight toml >}}
[storage.fs.eosgrpc]
slave_url = "root://eos-example.org"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="cache_directory" type="string" default="/var/tmp/" %}}
Location on the local fs where to store reads. Defaults to os.TempDir() [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eosgrpc/eosgrpc.go#L109)
{{< highlight toml >}}
[storage.fs.eosgrpc]
cache_directory = "/var/tmp/"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="sec_protocol" type="string" default="-" %}}
SecProtocol specifies the xrootd security protocol to use between the server and EOS. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eosgrpc/eosgrpc.go#L112)
{{< highlight toml >}}
[storage.fs.eosgrpc]
sec_protocol = "-"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="user_layout" type="string" default="-" %}}
UserLayout wraps the internal path with user information. Example: if conf.Namespace is /eos/user and received path is /docs and the UserLayout is {{.Username}} the internal path will be: /eos/user/<username>/docs [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eosgrpc/eosgrpc.go#L124)
{{< highlight toml >}}
[storage.fs.eosgrpc]
user_layout = "-"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="enable_logging" type="bool" default=false %}}
Enables logging of the commands executed Defaults to false [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eosgrpc/eosgrpc.go#L128)
{{< highlight toml >}}
[storage.fs.eosgrpc]
enable_logging = false
{{< /highlight >}}
{{% /dir %}}

{{% dir name="show_hidden_sys_files" type="bool" default=- %}}
ShowHiddenSysFiles shows internal EOS files like .sys.v# and .sys.a# files. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eosgrpc/eosgrpc.go#L132)
{{< highlight toml >}}
[storage.fs.eosgrpc]
show_hidden_sys_files = -
{{< /highlight >}}
{{% /dir %}}

{{% dir name="force_single_user_mode" type="bool" default=false %}}
ForceSingleUserMode will force connections to EOS to use SingleUsername [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eosgrpc/eosgrpc.go#L135)
{{< highlight toml >}}
[storage.fs.eosgrpc]
force_single_user_mode = false
{{< /highlight >}}
{{% /dir %}}

{{% dir name="UseKeytab" type="bool" default=false %}}
UseKeyTabAuth changes will authenticate requests by using an EOS keytab. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eosgrpc/eosgrpc.go#L138)
{{< highlight toml >}}
[storage.fs.eosgrpc]
UseKeytab = false
{{< /highlight >}}
{{% /dir %}}

{{% dir name="enable_home" type="bool" default=false %}}
EnableHome enables the creation of home directories. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eosgrpc/eosgrpc.go#L141)
{{< highlight toml >}}
[storage.fs.eosgrpc]
enable_home = false
{{< /highlight >}}
{{% /dir %}}

{{% dir name="authkey" type="string" default="-" %}}
Authkey is the key that authorizes this client to connect to the GRPC service It's unclear whether this will be the final solution [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eosgrpc/eosgrpc.go#L145)
{{< highlight toml >}}
[storage.fs.eosgrpc]
authkey = "-"
{{< /highlight >}}
{{% /dir %}}

