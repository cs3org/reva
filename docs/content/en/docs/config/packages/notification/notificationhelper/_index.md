---
title: "notificationhelper"
linkTitle: "notificationhelper"
weight: 10
description: >
  Configuration for the notificationhelper service
---

# _struct: Config_

{{% dir name="nats_address" type="string" default="" %}}
The NATS server address. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/notification/notificationhelper/notificationhelper.go#L47)
{{< highlight toml >}}
[notification.notificationhelper]
nats_address = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="nats_token" type="string" default="" %}}
The token to authenticate against the NATS server [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/notification/notificationhelper/notificationhelper.go#L48)
{{< highlight toml >}}
[notification.notificationhelper]
nats_token = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="nats_stream" type="string" default="reva-notifications" %}}
The notifications NATS stream. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/notification/notificationhelper/notificationhelper.go#L49)
{{< highlight toml >}}
[notification.notificationhelper]
nats_stream = "reva-notifications"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="templates" type="map[string]interface{}" default= %}}
Notification templates for the service. [[Ref]](https://github.com/cs3org/reva/tree/master/pkg/notification/notificationhelper/notificationhelper.go#L50)
{{< highlight toml >}}
[notification.notificationhelper]
templates = 
{{< /highlight >}}
{{% /dir %}}

