---
title: "ocmd"
linkTitle: "ocmd"
weight: 10
description: >
  Configuration for the ocmd service
---

# _struct: Config_

{{% dir name="nil" type="*smtpclient.SMTPCredentials" default="smtpclient" %}}
Configuration for mail settings [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/ocmd/ocmd.go#L39)
{{< highlight toml >}}
[http.services.ocmd.nil.smtpclient]
sender_login = ""
sender_mail = ""
sender_password = ""
smtp_server = ""
smtp_port = 587
disable_auth = false
local_name = ""

{{< /highlight >}}
{{% /dir %}}

{{% dir name="gatewaysvc" type="string" default="" %}}
Address of the gateway service. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/ocmd/ocmd.go#L42)
{{< highlight toml >}}
[http.services.ocmd]
gatewaysvc = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="mesh_directory_url" type="string" default="https://sciencemesh.cesnet.cz/iop/meshdir/" %}}
URL of the mesh directory url [[Ref]](https://github.com/cs3org/reva/tree/master/internal/http/services/ocmd/ocmd.go#L43)
{{< highlight toml >}}
[http.services.ocmd]
mesh_directory_url = "https://sciencemesh.cesnet.cz/iop/meshdir/"
{{< /highlight >}}
{{% /dir %}}

