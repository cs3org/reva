Enhancement: Allow to specify a shutdown timeout

When setting `graceful_shutdown_timeout` revad will try to shutdown in a
graceful manner when receiving an INT or TERM signal (similar to how it already
behaves on SIGQUIT). This allows ongoing operations to complete before exiting.

If the shutdown didn't finish before `graceful_shutdown_timeout` seconds the
process will exit with an error code (1).

https://github.com/cs3org/reva/pull/4072
