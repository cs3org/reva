Bugfix: make removal of favourites work

Currently, removing a folder from your favourites is broken, because the handleFavAttr method is only called in SetAttr, not in UnsetAttr. This change fixes this.

https://github.com/cs3org/reva/pull/4930