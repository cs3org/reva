Bugfix: check EOS errorcode on CreateHome

When checking whether a user already has a home, we did not specifically check
whether the error returned was NOT_FOUND, leading us to try to create a home when it may
already exist (which creates new backup jobs, etc.). Now, we only run CreateHome when EOS
reports NOT_FOUND

https://github.com/cs3org/reva/pull/5218