Bugfix: Make date only expiry dates valid for the whole day

When an expiry date like `2022-09-30` is parsed, we now make it valid for the whole day, effectively becoming `2022-09-30 23:59:59`

https://github.com/cs3org/reva/pull/3298
