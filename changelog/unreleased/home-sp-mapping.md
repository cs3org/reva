Enhancement: Functionality to map home directory to different storage providers

We hardcode the home path for all users to /home. This forbids redirecting
requests for different users to multiple storage providers. This PR provides the
option to map the home directories of different users using user attributes.

https://github.com/cs3org/reva/pull/1142
