Bugfix: Allow removing password from public links

When using cs3 public link share manager passwords would never be removed. We now remove the 
password when getting an update request with empty password field

https://github.com/cs3org/reva/pull/3094
