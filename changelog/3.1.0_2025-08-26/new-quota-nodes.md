Enhancement: add support for new EOS project quota nodes

For EOS projects, quota nodes used to be set under the service
account of the project on the path /eos/project

This has been changed to using GID=99 and having the path of the
project be the quota node

This change introduces support for the new system

https://github.com/cs3org/reva/pull/5278