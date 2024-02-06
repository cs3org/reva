Enhancement: force using MGM cache for finds

From EOS versions >= 5.2, "eos find" command
will query the QuarkDB node for information and 
not rely on cached information from the MGM.

We force to always use cached information as this
will speed up listing of containers.

https://github.com/cs3org/reva/pull/4500
