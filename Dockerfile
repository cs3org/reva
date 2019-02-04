FROM golang:1.11

ENV GO111MODULE=on
WORKDIR /go/src/github/cernbox/reva
COPY . .
RUN go build -v ./...
WORKDIR /go/src/github/cernbox/reva/cmd/revad
RUN go install
RUN mkdir -p /etc/revad/
RUN cp /go/src/github/cernbox/reva/cmd/revad/revad.toml /etc/revad/revad.toml
EXPOSE 9998
EXPOSE 9999
CMD ["/go/bin/revad", "-c", "/etc/revad/revad.toml", "-p", "/var/run/revad.pid"]
