---
title: "utils"
linkTitle: "utils"
weight: 10
description: >
  Configuration for the utils service
---

# _struct: LDAPConn_

{{% dir name="insecure" type="bool" default=false %}}
Whether to skip certificate checks when sending requests. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/utils/ldap.go#L36)
{{< highlight toml >}}
[utils]
insecure = false
{{< /highlight >}}
{{% /dir %}}

