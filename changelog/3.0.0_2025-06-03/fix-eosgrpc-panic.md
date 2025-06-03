Bugfix: eosgrpc: fixed panic with ACLs handling

Fixes a panic that happens when listing a folder where
files have no SysACLs, parent == nil and versionFolder has ACLs.

https://github.com/cs3org/reva/pull/5143
