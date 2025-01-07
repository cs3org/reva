Bugfix: ocm: fixed domain not having a protocol scheme

This PR fixes a bug in the OCM open driver that causes it to be unable to probe
OCM services at the remote server due to the domain having an unsupported
protocol scheme. in this case domain doesn't have a scheme and the changes in
this PR add a scheme to the domain before doing the probe.

https://github.com/cs3org/reva/pull/4790
