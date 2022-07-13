Bugfix: Make CS3 sharing drivers parse legacy resource id

The CS3 public and user sharing drivers will now correct a resource id that is missing a spaceid when it can split the storageid.

https://github.com/cs3org/reva/pull/3071
