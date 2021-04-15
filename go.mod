module github.com/cs3org/reva

require (
	contrib.go.opencensus.io/exporter/jaeger v0.2.1
	contrib.go.opencensus.io/exporter/prometheus v0.3.0
	github.com/BurntSushi/toml v0.3.1
	github.com/Masterminds/goutils v1.1.0 // indirect
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/Masterminds/sprig v2.22.0+incompatible
	github.com/ReneKroon/ttlcache/v2 v2.4.0
	github.com/aws/aws-sdk-go v1.38.13
	github.com/bluele/gcache v0.0.2 // indirect
	github.com/c-bata/go-prompt v0.2.5
	github.com/cheggaaa/pb v1.0.29
	github.com/coreos/go-oidc v2.2.1+incompatible
	github.com/cs3org/cato v0.0.0-20200828125504-e418fc54dd5e
	github.com/cs3org/go-cs3apis v0.0.0-20210325133324-32b03d75a535
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/eventials/go-tus v0.0.0-20200718001131-45c7ec8f5d59
	github.com/go-ldap/ldap/v3 v3.3.0
	github.com/go-openapi/errors v0.19.6 // indirect
	github.com/go-openapi/strfmt v0.19.2 // indirect
	github.com/go-sql-driver/mysql v1.6.0
	github.com/golang/protobuf v1.5.2
	github.com/gomodule/redigo v1.8.4
	github.com/google/go-github v17.0.0+incompatible
	github.com/google/go-querystring v1.0.0 // indirect
	github.com/google/uuid v1.2.0
	github.com/grpc-ecosystem/go-grpc-middleware v1.3.0
	github.com/huandu/xstrings v1.3.0 // indirect
	github.com/imdario/mergo v0.3.8 // indirect
	github.com/jedib0t/go-pretty v4.3.0+incompatible
	github.com/mattn/go-sqlite3 v2.0.3+incompatible
	github.com/minio/minio-go/v7 v7.0.10
	github.com/mitchellh/copystructure v1.0.0 // indirect
	github.com/mitchellh/mapstructure v1.4.1
	github.com/onsi/ginkgo v1.16.1
	github.com/onsi/gomega v1.11.0
	github.com/ory/fosite v0.39.0
	github.com/pkg/errors v0.9.1
	github.com/pkg/xattr v0.4.3
	github.com/pquerna/cachecontrol v0.0.0-20180517163645-1555304b9b35 // indirect
	github.com/rs/cors v1.7.0
	github.com/rs/zerolog v1.21.0
	github.com/sciencemesh/meshdirectory-web v1.0.4
	github.com/stretchr/testify v1.7.0
	github.com/studio-b12/gowebdav v0.0.0-20200303150724-9380631c29a1
	github.com/tus/tusd v1.1.1-0.20200416115059-9deabf9d80c2
	go.opencensus.io v0.23.0
	golang.org/x/crypto v0.0.0-20201203163018-be400aefbc4c
	golang.org/x/net v0.0.0-20201202161906-c7110b5ffcbb
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d
	golang.org/x/sys v0.0.0-20210218155724-8ebf48af031b
	golang.org/x/term v0.0.0-20201117132131-f5c789dd3221
	google.golang.org/grpc v1.37.0
	google.golang.org/protobuf v1.26.0
	gotest.tools v2.2.0+incompatible
)

go 1.16

replace (
	github.com/eventials/go-tus => github.com/andrewmostello/go-tus v0.0.0-20200314041820-904a9904af9a
	github.com/oleiade/reflections => github.com/oleiade/reflections v1.0.1
	google.golang.org/grpc => google.golang.org/grpc v1.26.0 // temporary downgrade
)
