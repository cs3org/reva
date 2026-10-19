Enhancement: Typed invocation arguments

Invocation arguments are declared with their type (`invoke.OptArg[int]`, `invoke.Arg[time.Time]`, ...) and parsed by the framework before the handler runs, so a bad value is rejected instead of read as zero. A time argument accepts RFC3339 or a duration meaning that long ago.

https://github.com/cs3org/reva/pull/5925
