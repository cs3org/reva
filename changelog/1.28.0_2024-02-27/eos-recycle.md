Enhancement: Limit max number of entries returned by ListRecycle in eos

The idea is to query first how many entries we'd have from eos recycle ls and bail out if "too many".

https://github.com/cs3org/reva/pull/4455
