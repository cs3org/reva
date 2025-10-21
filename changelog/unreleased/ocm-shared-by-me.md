Enhancement: Include OCM shares in SharedByMe view

- The CS3APis verison has been updated to include "ListExistingOcmShares".
- The OCM shares are now included in the getSharedByMe call.
- The filters have been updated to adapt to changes from the updated CS3APIs.
- Fixed bug where only ocm users were queried if it was enabled.


https://github.com/cs3org/reva/pull/5363