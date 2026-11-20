Enhancement: Limit the number of cephmount user threads

The new option `max_user_threads` (default 1000) limits the threads of each cephmount driver. A thread counts until it exits. When the limit is reached, the idle thread that was used longest ago stops. If no thread is idle, the request fails.

https://github.com/cs3org/reva/pull/5970