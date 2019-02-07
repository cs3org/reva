package loader

import (
	// Load core authentication managers.
	_ "github.com/cernbox/reva/pkg/auth/manager/demo"
	_ "github.com/cernbox/reva/pkg/auth/manager/impersonator"
	_ "github.com/cernbox/reva/pkg/auth/manager/ldap"
	// Add your own here
)
