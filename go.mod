module github.com/cs3org/reva

require (
	bou.ke/monkey v1.0.2
	contrib.go.opencensus.io/exporter/prometheus v0.4.1
	github.com/BurntSushi/toml v1.1.0
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/Masterminds/sprig v2.22.0+incompatible
	github.com/ReneKroon/ttlcache/v2 v2.11.0
	github.com/asim/go-micro/plugins/events/nats/v4 v4.0.0-20220118152736-9e0be6c85d75
	github.com/aws/aws-sdk-go v1.44.27
	github.com/beevik/etree v1.1.0
	github.com/bluele/gcache v0.0.2
	github.com/c-bata/go-prompt v0.2.5
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/ceph/go-ceph v0.15.0
	github.com/cheggaaa/pb v1.0.29
	github.com/coreos/go-oidc v2.2.1+incompatible
	github.com/cs3org/cato v0.0.0-20200828125504-e418fc54dd5e
	github.com/cs3org/go-cs3apis v0.0.0-20221004162747-f20ee4756d90
	github.com/cubewise-code/go-mime v0.0.0-20200519001935-8c5762b177d8
	github.com/dgraph-io/ristretto v0.1.0
	github.com/eventials/go-tus v0.0.0-20200718001131-45c7ec8f5d59
	github.com/gdexlab/go-render v1.0.1
	github.com/go-chi/chi/v5 v5.0.7
	github.com/go-ldap/ldap/v3 v3.4.3
	github.com/go-sql-driver/mysql v1.6.0
	github.com/golang-jwt/jwt v3.2.2+incompatible
	github.com/golang/protobuf v1.5.2
	github.com/gomodule/redigo v1.8.8
	github.com/google/go-cmp v0.5.8
	github.com/google/go-github v17.0.0+incompatible
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/uuid v1.3.0
	github.com/grpc-ecosystem/go-grpc-middleware v1.3.0
	github.com/hashicorp/go-hclog v1.2.1
	github.com/hashicorp/go-plugin v1.4.4
	github.com/huandu/xstrings v1.3.2 // indirect
	github.com/jedib0t/go-pretty v4.3.0+incompatible
	github.com/juliangruber/go-intersect v1.1.0
	github.com/mattn/go-sqlite3 v1.14.10
	github.com/maxymania/go-system v0.0.0-20170110133659-647cc364bf0b
	github.com/mileusna/useragent v1.0.2
	github.com/minio/minio-go/v7 v7.0.40
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/go-testing-interface v1.14.1 // indirect
	github.com/mitchellh/mapstructure v1.5.0
	github.com/nats-io/nats-server/v2 v2.8.4
	github.com/nats-io/nats-streaming-server v0.24.6
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/gomega v1.19.0
	github.com/pkg/errors v0.9.1
	github.com/pkg/xattr v0.4.5
	github.com/pquerna/cachecontrol v0.1.0 // indirect
	github.com/prometheus/alertmanager v0.24.0
	github.com/rs/cors v1.8.2
	github.com/rs/zerolog v1.27.0
	github.com/sciencemesh/meshdirectory-web v1.0.4
	github.com/sethvargo/go-password v0.2.0
	github.com/stretchr/testify v1.7.2
	github.com/studio-b12/gowebdav v0.0.0-20210917133250-a3a86976a1df
	github.com/thanhpk/randstr v1.0.4
	github.com/tidwall/pretty v1.2.0 // indirect
	github.com/tus/tusd v1.9.0
	github.com/wk8/go-ordered-map v1.0.0
	go-micro.dev/v4 v4.3.1-0.20211108085239-0c2041e43908
	go.opencensus.io v0.23.0
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.32.0
	go.opentelemetry.io/otel v1.7.0
	go.opentelemetry.io/otel/exporters/jaeger v1.7.0
	go.opentelemetry.io/otel/sdk v1.7.0
	go.opentelemetry.io/otel/trace v1.7.0
	golang.org/x/crypto v0.0.0-20220722155217-630584e8d5aa
	golang.org/x/oauth2 v0.0.0-20220309155454-6242fa91716a
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20220722155257-8c9f86f7a55f
	golang.org/x/term v0.0.0-20210927222741-03fcf44c2211
	google.golang.org/genproto v0.0.0-20220324131243-acbaeb5b85eb
	google.golang.org/grpc v1.47.0
	google.golang.org/protobuf v1.28.0
	gopkg.in/square/go-jose.v2 v2.6.0 // indirect
	gotest.tools v2.2.0+incompatible
)

go 1.16

replace (
	github.com/eventials/go-tus => github.com/andrewmostello/go-tus v0.0.0-20200314041820-904a9904af9a
	github.com/oleiade/reflections => github.com/oleiade/reflections v1.0.1
)
