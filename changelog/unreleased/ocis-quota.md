Enhancement: quota querying and tree accounting

The ocs api now returns the user quota for the users home storage. Furthermore, the ocis storage driver now reads the quota from the extended attributes of the user home or root node and implements tree size accounting. Finally, ocdav PROPFINDS now handle the `DAV:quota-used-bytes` and `DAV:quote-available-bytes` properties.

https://github.com/cs3org/reva/pull/1405
https://github.com/cs3org/reva/pull/1491
