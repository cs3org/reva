Enhancement: Init time logger for eosgrpc storage driver

Before the `eosgrpc` driver was using a custom logger.
Now that the reva logger is available at init time,
the driver will use this.

https://github.com/cs3org/reva/pull/4311
