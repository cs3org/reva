---
title: "Log"
linkTitle: "Log"
weight: 5
description: >
  Directives to configure the logging system
---

{{% dir name="mode" type="string" default="console" %}}
Specifies the format of the logs. `consoles` writes logs to be consumed by humans.
`json` writes logs in the JSON format for be consumed by machines.
{{< highlight toml >}}
[log]
mode = "json"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="level" type="string" default="info" %}}
Specifies the log level. Valid values are `debug`, `info`, `warn` and `error`.
{{< highlight toml >}}
[log]
level = "debug"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="output" type="string" default="stdout" %}}
Specifies the log output. Special values `stdout` will write to standard output and `stderr` to standard error.
Any other value write to a file. If the file already exists it will append to it.
{{< highlight toml >}}
[log]
output = "/var/log/revad.log"
{{< /highlight >}}
{{% /dir %}}
