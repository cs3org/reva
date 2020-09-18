Bugfix: Call the gateway stat method from appprovider

The appprovider service used to directly pass the stat request to the storage
provider bypassing the gateway, which resulted in errors while handling share
children as they are resolved in the gateway path.

https://github.com/cs3org/reva/pull/1140
