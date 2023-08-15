Enhancement: Notifications framework

Adds a notifications framework to Reva.

The new notifications service communicates with the rest of
reva using NATS. It provides helper functions to register new
notifications and to send them.

Notification templates are provided in the configuration files
for each service, and they are registered into the notifications
service on initialization.

https://github.com/cs3org/reva/pull/3825
