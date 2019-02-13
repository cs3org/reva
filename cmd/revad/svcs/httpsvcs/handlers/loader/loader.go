package loader

import (
	// Load core HTTP middlewares.
	_ "github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/handlers/auth"
	_ "github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/handlers/cors"
	_ "github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/handlers/log"
	_ "github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/handlers/trace"
	// Add your own middlware.
)
