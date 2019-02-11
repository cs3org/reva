package loader

import (
	// Load core gRPC services.
	_ "github.com/cernbox/reva/cmd/revad/svcs/grpcsvcs/appprovidersvc"
	_ "github.com/cernbox/reva/cmd/revad/svcs/grpcsvcs/appregistrysvc"
	_ "github.com/cernbox/reva/cmd/revad/svcs/grpcsvcs/authsvc"
	_ "github.com/cernbox/reva/cmd/revad/svcs/grpcsvcs/storagebrokersvc"
	_ "github.com/cernbox/reva/cmd/revad/svcs/grpcsvcs/storageprovidersvc"
	// Add your own service here
)
