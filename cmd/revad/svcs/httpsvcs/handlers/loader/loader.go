package loader

import (
	// Load core HTTP middlewares.
	_ "github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/handlers/auth"
	_ "github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/handlers/cors"
	// Add your own middlware.
)
