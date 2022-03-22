package main

import (
	"fmt"
	"os"

	"github.com/cs3org/reva/v2/pkg/micro/ocdav"
	"github.com/rs/zerolog"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

	s, err := ocdav.Service(
		ocdav.Logger(logger),
		ocdav.GatewaySvc("127.0.0.1:9142"),
		ocdav.FilesNamespace("/users/{{.Id.OpaqueId}}"),
		ocdav.WebdavNamespace("/users/{{.Id.OpaqueId}}"),
		ocdav.SharesNamespace("/Shares"),
	)
	if err != nil {
		fmt.Printf(err.Error())
		return
	}
	s.Run()
}
