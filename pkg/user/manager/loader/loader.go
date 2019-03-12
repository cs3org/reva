package loader

import (
	// Load core user manager drivers.
	_ "github.com/cernbox/reva/pkg/user/manager/demo"
	_ "github.com/cernbox/reva/pkg/user/manager/ldap"
	_ "github.com/cernbox/reva/pkg/user/manager/oidc"
	// Add your own here
)
