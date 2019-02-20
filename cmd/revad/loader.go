package main

import (
	// These are all the extensions points for REVA
	_ "github.com/cernbox/reva/cmd/revad/svcs/grpcsvcs/interceptors/loader"
	_ "github.com/cernbox/reva/cmd/revad/svcs/grpcsvcs/loader"
	_ "github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/handlers/auth/credential/loader"
	_ "github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/handlers/auth/token/loader"
	_ "github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/handlers/auth/tokenwriter/loader"
	_ "github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/handlers/loader"
	_ "github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/loader"
	_ "github.com/cernbox/reva/pkg/auth/manager/loader"
	_ "github.com/cernbox/reva/pkg/storage/broker/loader"
	_ "github.com/cernbox/reva/pkg/storage/fs/loader"
	_ "github.com/cernbox/reva/pkg/token/manager/loader"
	_ "github.com/cernbox/reva/pkg/user/manager/loader"
)
