Enhancement: Pass lock holder metadata on uploads

We now pass relevant metadata (lock id and lock holder) downstream
on uploads, and handle the case of conflicts due to lock mismatch.

https://github.com/cs3org/reva/pull/4514
