Enhancement: Lazy initialize public share manager

Unlike the share manager the public share manager was initializing its data structure on startup. This can lead to failed ocis
starts (in single binary case) or to restarting `sharing` pods when running in containerized environment.

https://github.com/cs3org/reva/pull/4490
