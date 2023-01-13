Enhancement: Pass estream to Storage Providers

Similar to the data providers we now pass the stream to the `New` func. This will reduce connections from storage providers to nats.

https://github.com/cs3org/reva/pull/3598
