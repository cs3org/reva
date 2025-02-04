Bugfix: impersonate owner on ListRevisions

ListRevisions is currently broken for projects, because this happens on behalf of the user, instead of the owner of the file. This behaviour is changed to do the call on behalf of the owner (if we are in a non-home space).

https://github.com/cs3org/reva/pull/5064