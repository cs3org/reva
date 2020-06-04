module github.com/cs3org/reva

require (
	contrib.go.opencensus.io/exporter/jaeger v0.2.0
	contrib.go.opencensus.io/exporter/prometheus v0.2.0
	github.com/BurntSushi/toml v0.3.1
	github.com/Masterminds/goutils v1.1.0 // indirect
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/Masterminds/sprig v2.22.0+incompatible
	github.com/aws/aws-sdk-go v1.31.10
	github.com/cheggaaa/pb v1.0.28
	github.com/coreos/go-oidc v2.2.1+incompatible
	github.com/cs3org/go-cs3apis v0.0.0-20200515145316-7048e6a5a73d
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/eventials/go-tus v0.0.0-20190617130015-9db47421f6a0
	github.com/go-openapi/strfmt v0.19.2 // indirect
	github.com/gofrs/uuid v3.3.0+incompatible
	github.com/golang/protobuf v1.4.2
	github.com/gomodule/redigo v2.0.0+incompatible
	github.com/google/uuid v1.1.1
	github.com/grpc-ecosystem/go-grpc-middleware v1.2.0
	github.com/huandu/xstrings v1.3.0 // indirect
	github.com/imdario/mergo v0.3.8 // indirect
	github.com/jedib0t/go-pretty v4.3.0+incompatible
	github.com/mattn/go-runewidth v0.0.4 // indirect
	github.com/mattn/go-sqlite3 v2.0.3+incompatible
	github.com/mitchellh/copystructure v1.0.0 // indirect
	github.com/mitchellh/mapstructure v1.3.1
	github.com/ory/fosite v0.32.0
	github.com/pkg/errors v0.9.1
	github.com/pkg/xattr v0.4.1
	github.com/pquerna/cachecontrol v0.0.0-20180517163645-1555304b9b35 // indirect
	github.com/rs/cors v1.7.0
	github.com/rs/zerolog v1.19.0
	github.com/tus/tusd v1.1.1-0.20200416115059-9deabf9d80c2
	go.opencensus.io v0.22.3
	golang.org/x/crypto v0.0.0-20191011191535-87dc89f01550
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d
	google.golang.org/grpc v1.29.1
	gopkg.in/asn1-ber.v1 v1.0.0-20181015200546-f715ec2f112d // indirect
	gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f // indirect
	gopkg.in/cheggaaa/pb.v1 v1.0.27 // indirect
	gopkg.in/ldap.v2 v2.5.1
)

go 1.13

replace github.com/eventials/go-tus => github.com/andrewmostello/go-tus v0.0.0-20200314041820-904a9904af9a
