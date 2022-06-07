Bugfix: the decomposedfs now checks the GetPath permission

After fixing the meta endpoint and introducing the fieldmask the GetPath call is made directly to the storageprovider. The decomposedfs now checks if the current user actually has the permission to get the path. Before the two previous PRs this was covered by the list storage spaces call which used a stat request and the stat permission. 

https://github.com/cs3org/reva/pull/2909