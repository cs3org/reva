---
title: "notifications"
linkTitle: "notifications"
weight: 10
description: >
  Configuration for the notifications service
---

# _struct: config_

{{% dir name="nats_address" type="string" default="" %}}
The NATS server address. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/serverless/services/notifications/notifications.go#L47)
{{< highlight toml >}}
[serverless.services.notifications]
nats_address = ""
{{< /highlight >}}
{{% /dir %}}

{{% dir name="nats_token" type="string" default="The token to authenticate against the NATS server" %}}
 [[Ref]](https://github.com/cs3org/reva/tree/master/internal/serverless/services/notifications/notifications.go#L48)
{{< highlight toml >}}
[serverless.services.notifications]
nats_token = "The token to authenticate against the NATS server"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="nats_prefix" type="string" default="reva-notifications" %}}
The notifications NATS stream. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/serverless/services/notifications/notifications.go#L49)
{{< highlight toml >}}
[serverless.services.notifications]
nats_prefix = "reva-notifications"
{{< /highlight >}}
{{% /dir %}}

{{% dir name="handlers" type="map[string]map[string]interface{}" default= %}}
Settings for the different notification handlers. [[Ref]](https://github.com/cs3org/reva/tree/master/internal/serverless/services/notifications/notifications.go#L50)
{{< highlight toml >}}
[serverless.services.notifications]
handlers = 
{{< /highlight >}}
{{% /dir %}}

{{% dir name="grouping_interval" type="int" default=60 %}}
Time in seconds to group incoming notification triggers [[Ref]](https://github.com/cs3org/reva/tree/master/internal/serverless/services/notifications/notifications.go#L51)
{{< highlight toml >}}
[serverless.services.notifications]
grouping_interval = 60
{{< /highlight >}}
{{% /dir %}}

{{% dir name="grouping_max_size" type="int" default=100 %}}
Maximum number of notifications to group [[Ref]](https://github.com/cs3org/reva/tree/master/internal/serverless/services/notifications/notifications.go#L52)
{{< highlight toml >}}
[serverless.services.notifications]
grouping_max_size = 100
{{< /highlight >}}
{{% /dir %}}

{{% dir name="storage_driver" type="string" default="mysql" %}}
The driver used to store notifications [[Ref]](https://github.com/cs3org/reva/tree/master/internal/serverless/services/notifications/notifications.go#L53)
{{< highlight toml >}}
[serverless.services.notifications]
storage_driver = "mysql"
{{< /highlight >}}
{{% /dir %}}

