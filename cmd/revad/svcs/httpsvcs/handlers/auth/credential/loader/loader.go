package loader

import (
	// Load core authentication strategies.
	_ "github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/handlers/auth/credential/strategy/basic"
	_ "github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/handlers/auth/credential/strategy/oidc"
	// Add your own here.
)
