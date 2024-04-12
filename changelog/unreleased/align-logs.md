Bugfix: consistently use spaceid and nodeid in logs

Sometimes we tried to log a node which led to a JSON recursion error because it contains a reference to the space root, which references itself. We now always log `spaceid` and `nodeid`.

https://github.com/cs3org/reva/pull/4623
