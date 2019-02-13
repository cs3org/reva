package loader

import (
	// Load core interceptors.
	_ "github.com/cernbox/reva/cmd/revad/svcs/grpcsvcs/interceptors/auth"
	_ "github.com/cernbox/reva/cmd/revad/svcs/grpcsvcs/interceptors/log"
	_ "github.com/cernbox/reva/cmd/revad/svcs/grpcsvcs/interceptors/prometheus"
	_ "github.com/cernbox/reva/cmd/revad/svcs/grpcsvcs/interceptors/recovery"
	_ "github.com/cernbox/reva/cmd/revad/svcs/grpcsvcs/interceptors/trace"
	// Add your own.
)
