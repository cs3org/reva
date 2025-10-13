Bugfix: fix EOS tokens + EOS 5.3.22

Since EOS 5.3.22 does more strict checks on tokens, we:
*  Remove "x" bit from permissions in EOS token
*  assume role instead of using token when restoring / listing / downloading revisions

https://github.com/cs3org/reva/pull/5335
