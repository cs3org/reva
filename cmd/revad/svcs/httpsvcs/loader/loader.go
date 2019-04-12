package loader

import (
	// Load core HTTP services
	_ "github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/datasvc"
	_ "github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/helloworldsvc"
	_ "github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/iframeuisvc"
	_ "github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/ocdavsvc"
	_ "github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/prometheussvc"
	_ "github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/webuisvc"
	// Add your own service here
)
