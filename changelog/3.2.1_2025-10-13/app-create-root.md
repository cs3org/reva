Bugfix: Support requests to /app/new on space root without container ID

For some reason, the web client was not sending the container ID when in a space root,
so we also accept this (since we can deduce it from the Space ID)

https://github.com/cs3org/reva/pull/5340
