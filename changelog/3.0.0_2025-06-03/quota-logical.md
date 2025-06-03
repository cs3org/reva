Bugfix: use logicalbytes instead of bytes

EOS gRPC used `usedbytes` instead of `usedlogicalbytes` for calculating quota, resulting in a wrong view

https://github.com/cs3org/reva/pull/5084