Bugfix: fix strconv.Atoi overflow on 32-bit platforms  
  
Replaced strconv.Atoi with strconv.ParseInt(..., 10, 64) to avoid  
overflow issues on 32-bit platforms.  
  
https://github.com/cs3org/reva/pull/5751
