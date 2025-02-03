Bugfix: impersonate owner on ListRevisions

ListRevisions is currently broken for projects, because this happens on behalf of the user, instead of the owner of the file. This behaviour is changed to, if so configured, do the call on behalf of the owner.

https://github.com/cs3org/reva/pull/5064