Bugfix: Carefully use root credentials to perform system level ops

This PR ensures that system level ops like setlock, setattr, stat... work when invoked from a gateway
This is relevant for eosgrpc, as eosbinary exploited the permissivity of the eos cmdline

https://github.com/cs3org/reva/pull/4369
