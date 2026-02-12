Bugfix: do not clean namespace path

There was a bug in Reva that caused the namespace path to be cleaned. This is wrong: this path should end in a `/` so that EOS trashbin paths do not match for projects (e.g. `/eos/project-i00` should not be prefixed by `/eos/project/`).

This bug caused deleted entries to still show up in the favorites list

https://github.com/cs3org/reva/pull/5491
