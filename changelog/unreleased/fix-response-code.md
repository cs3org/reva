Bugfix: Return proper response code when detecting recursive copy/move operations

We changed the ocdav response code to "409 - Conflict" when a recursive operation was detected.

https://github.com/cs3org/reva/pull/3031
