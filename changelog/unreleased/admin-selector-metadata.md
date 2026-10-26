Enhancement: select admin targets by registry metadata

The Admin API selector accepts `service[key=value,...]`, which matches the live
instances of the service whose registry metadata has all the pairs. A shared
invocation goes to one of the matches, any other invocation to each of them.
The storage provider now advertises its `mount_id` and `mount_path` in the
registry, so `storageprovider[mount_id=eoshome-i01]` reaches the instance that
serves a mount without a node id.

https://github.com/cs3org/reva/pull/XXXX
