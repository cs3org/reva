Bugfix: wait for nats server on middleware start

Use a retry mechanism to connect to the nats server when it is not ready yet

https://github.com/cs3org/reva/pull/2572
