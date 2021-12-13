Enhancement: Removed DELETE_USER_DATA_CMD from tests
    
With recent changes in the test configs for user and group providers we
no longer need to manually remove the user data after each testcase.
User and groups will get a new unique id with every test run.

https://github.com/cs3org/reva/pull/2367
