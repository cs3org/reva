package loader

import (
	// Load core token managers.
	_ "github.com/cernbox/reva/pkg/token/manager/demo"
	_ "github.com/cernbox/reva/pkg/token/manager/jwt"
	// Add your own here.
)
