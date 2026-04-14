---
title: "eos"
linkTitle: "eos"
weight: 10
description: >
  Configuration for the eos service
---

# _struct: Config_

{{% dir name="external_accounts_user_name" type="string" default="nil" %}}
Username of a user known to EOS on whose behalf accesses are done for external (lw, federated) users. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eos/config.go#L178)
{{< highlight toml >}}
[storage.fs.eos]
external_accounts_user_name = "nil"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="external_accounts_user_uid" type="string" default="nil" %}}
UID of a user known to EOS on whose behalf accesses are done for external (lw, federated) users. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eos/config.go#L179)
{{< highlight toml >}}
[storage.fs.eos]
external_accounts_user_uid = "nil"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="external_accounts_user_gid" type="string" default="nil" %}}
GID of a user known to EOS on whose behalf accesses are done for external (lw, federated) users. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/storage/fs/eos/config.go#L180)
{{< highlight toml >}}
[storage.fs.eos]
external_accounts_user_gid = "nil"
{{< /highlight >}}
{{% /dir %}}

