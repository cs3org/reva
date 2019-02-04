FROM golang:1.11

ENV GO111MODULE=on
WORKDIR /go/src/github/cernbox/reva
COPY . .
RUN go build -v ./...
WORKDIR /go/src/github/cernbox/reva/cmd/revad
RUN go install
EXPOSE 9998
EXPOSE 9999
CMD ["/go/bin/revad", "-c", "/go/src/github/cernbox/reva/cmd/revad/revad.toml", "-p", "/var/run/revad.pid"]
