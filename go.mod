module github.com/cs3org/reva

require (
	contrib.go.opencensus.io/exporter/jaeger v0.2.1
	contrib.go.opencensus.io/exporter/prometheus v0.2.0
	github.com/BurntSushi/toml v0.3.1
	github.com/Masterminds/goutils v1.1.0 // indirect
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/Masterminds/sprig v2.22.0+incompatible
	github.com/aws/aws-sdk-go v1.35.9
	github.com/c-bata/go-prompt v0.2.5
	github.com/cheggaaa/pb v1.0.29
	github.com/coreos/go-oidc v2.2.1+incompatible
	github.com/cs3org/cato v0.0.0-20200828125504-e418fc54dd5e
	github.com/cs3org/go-cs3apis v0.0.0-20200929101248-821df597ec8d
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/eventials/go-tus v0.0.0-20200718001131-45c7ec8f5d59
	github.com/go-ldap/ldap/v3 v3.2.4
	github.com/go-openapi/errors v0.19.6 // indirect
	github.com/go-openapi/strfmt v0.19.2 // indirect
	github.com/gofrs/uuid v3.3.0+incompatible
	github.com/golang/protobuf v1.4.3
	github.com/gomodule/redigo v1.8.2
	github.com/google/go-github v17.0.0+incompatible
	github.com/google/go-querystring v1.0.0 // indirect
	github.com/google/uuid v1.1.2
	github.com/grpc-ecosystem/go-grpc-middleware v1.2.2
	github.com/huandu/xstrings v1.3.0 // indirect
	github.com/imdario/mergo v0.3.8 // indirect
	github.com/jedib0t/go-pretty v4.3.0+incompatible
	github.com/mattn/go-sqlite3 v2.0.3+incompatible
	github.com/mitchellh/copystructure v1.0.0 // indirect
	github.com/mitchellh/mapstructure v1.3.3
	github.com/ory/fosite v0.35.1
	github.com/pkg/errors v0.9.1
	github.com/pkg/xattr v0.4.1
	github.com/pquerna/cachecontrol v0.0.0-20180517163645-1555304b9b35 // indirect
	github.com/rs/cors v1.7.0
	github.com/rs/zerolog v1.20.0
	github.com/stretchr/testify v1.6.1
	github.com/studio-b12/gowebdav v0.0.0-20200303150724-9380631c29a1
	github.com/tus/tusd v1.1.1-0.20200416115059-9deabf9d80c2
	go.opencensus.io v0.22.5
	golang.org/x/crypto v0.0.0-20200709230013-948cd5f35899
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d
	google.golang.org/grpc v1.33.1
)

go 1.13

replace github.com/eventials/go-tus => github.com/andrewmostello/go-tus v0.0.0-20200314041820-904a9904af9a

replace google.golang.org/grpc => google.golang.org/grpc v1.26.0 // temporary downgrade
