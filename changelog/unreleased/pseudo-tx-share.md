Enhancement: pseudo-transactionalize sharing

Currently, sharing is not transactionalized: setting ACLs and writing the share to the db is completely independent. In the current situation, shares are written to the db before setting the ACL, letting users falsely believe that they successfully shared a resource, even if setting the ACL afterwards fails. his enhancement improves the situation by doing the least reliable (setting ACLs on EOS) first:
a) first pinging the db
b) writing the ACLs
c) writing to the db

https://github.com/cs3org/reva/pull/5029