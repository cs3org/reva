Bugfix: Handle error when user fetch fails in REST user manager

## Description

The current implementation of the REST user manager does not handle errors that occur when fetching user data from the remote server. As a result, if the fetch fails, errors are not returned, and the user manager continues to operate as if the fetch was successful.

This PR adds error handling for cases when the fetch fails. Specifically, the **`fetchAllUserAccounts`** function now returns an error if the fetch operation fails, and this error is propagated up to the caller.

https://github.com/cs3org/reva/pull/3790
