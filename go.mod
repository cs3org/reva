module github.com/owncloud/reva/v2

go 1.24

require (
	bou.ke/monkey v1.0.2
	contrib.go.opencensus.io/exporter/prometheus v0.4.2
	github.com/BurntSushi/toml v1.4.0
	github.com/CiscoM31/godata v1.0.11
	github.com/Masterminds/sprig v2.22.0+incompatible
	github.com/armon/go-radix v1.0.0
	github.com/aws/aws-sdk-go v1.55.7
	github.com/beevik/etree v1.5.1
	github.com/bluele/gcache v0.0.2
	github.com/c-bata/go-prompt v0.2.6
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/ceph/go-ceph v0.34.0
	github.com/cheggaaa/pb v1.0.29
	github.com/coreos/go-oidc/v3 v3.14.1
	github.com/cs3org/cato v0.0.0-20200828125504-e418fc54dd5e
	github.com/cs3org/go-cs3apis v0.0.0-20241105092511-3ad35d174fc1
	github.com/dgraph-io/ristretto v0.2.0
	github.com/emvi/iso-639-1 v1.1.0
	github.com/eventials/go-tus v0.0.0-20220610120217-05d0564bb571
	github.com/gdexlab/go-render v1.0.1
	github.com/go-chi/chi/v5 v5.2.2
	github.com/go-chi/render v1.0.3
	github.com/go-ldap/ldap/v3 v3.4.11
	github.com/go-micro/plugins/v4/events/natsjs v1.2.2
	github.com/go-micro/plugins/v4/server/http v1.2.2
	github.com/go-micro/plugins/v4/store/nats-js v1.2.1
	github.com/go-micro/plugins/v4/store/nats-js-kv v0.0.0-20240726082623-6831adfdcdc4
	github.com/go-micro/plugins/v4/store/redis v1.2.1
	github.com/go-playground/locales v0.14.1
	github.com/go-playground/universal-translator v0.18.1
	github.com/go-playground/validator/v10 v10.27.0
	github.com/go-redis/redis/v8 v8.11.5
	github.com/go-sql-driver/mysql v1.8.1
	github.com/gofrs/flock v0.12.1
	github.com/golang-jwt/jwt/v5 v5.2.2
	github.com/golang/protobuf v1.5.4
	github.com/gomodule/redigo v1.9.2
	github.com/google/go-cmp v0.7.0
	github.com/google/go-github v17.0.0+incompatible
	github.com/google/renameio/v2 v2.0.0
	github.com/google/uuid v1.6.0
	github.com/grpc-ecosystem/go-grpc-middleware v1.4.0
	github.com/hashicorp/go-hclog v1.6.3
	github.com/hashicorp/go-plugin v1.6.2
	github.com/huandu/xstrings v1.5.0
	github.com/iancoleman/strcase v0.3.0
	github.com/jedib0t/go-pretty v4.3.0+incompatible
	github.com/jellydator/ttlcache/v2 v2.11.1
	github.com/juliangruber/go-intersect v1.1.0
	github.com/mattn/go-sqlite3 v1.14.28
	github.com/maxymania/go-system v0.0.0-20170110133659-647cc364bf0b
	github.com/mileusna/useragent v1.3.5
	github.com/minio/minio-go/v7 v7.0.94
	github.com/mitchellh/mapstructure v1.5.0
	github.com/nats-io/nats-server/v2 v2.10.27
	github.com/nats-io/nats.go v1.43.0
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/ginkgo/v2 v2.23.3
	github.com/onsi/gomega v1.37.0
	github.com/owncloud/ocis/v2 v2.0.1-0.20250122072425-665072d86d1a
	github.com/pablodz/inotifywaitgo v0.0.7
	github.com/pkg/errors v0.9.1
	github.com/pkg/xattr v0.4.10
	github.com/prometheus/alertmanager v0.27.0
	github.com/prometheus/client_golang v1.20.5
	github.com/rogpeppe/go-internal v1.14.1
	github.com/rs/cors v1.11.1
	github.com/rs/zerolog v1.33.0
	github.com/segmentio/kafka-go v0.4.47
	github.com/sercand/kuberesolver/v5 v5.1.1
	github.com/sethvargo/go-password v0.3.1
	github.com/shamaton/msgpack/v2 v2.2.2
	github.com/shirou/gopsutil v3.21.11+incompatible
	github.com/stretchr/testify v1.10.0
	github.com/studio-b12/gowebdav v0.9.0
	github.com/test-go/testify v1.1.4
	github.com/thanhpk/randstr v1.0.6
	github.com/tus/tusd/v2 v2.6.0
	github.com/wk8/go-ordered-map v1.0.0
	go-micro.dev/v4 v4.11.0
	go.etcd.io/etcd/client/v3 v3.5.16
	go.opencensus.io v0.24.0
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.61.0
	go.opentelemetry.io/otel v1.37.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.37.0
	go.opentelemetry.io/otel/sdk v1.37.0
	go.opentelemetry.io/otel/trace v1.37.0
	golang.org/x/crypto v0.39.0
	golang.org/x/exp v0.0.0-20241009180824-f66d83c29e7c
	golang.org/x/oauth2 v0.30.0
	golang.org/x/sync v0.15.0
	golang.org/x/sys v0.33.0
	golang.org/x/term v0.32.0
	golang.org/x/text v0.26.0
	google.golang.org/genproto v0.0.0-20241015192408-796eee8c2d53
	google.golang.org/grpc v1.73.0
	google.golang.org/protobuf v1.36.6
)

require (
	dario.cat/mergo v1.0.1 // indirect
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/Azure/go-ntlmssp v0.0.0-20221128193559-754e69321358 // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/ProtonMail/go-crypto v1.1.3 // indirect
	github.com/ajg/form v1.5.1 // indirect
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bitly/go-simplejson v0.5.0 // indirect
	github.com/cenkalti/backoff/v5 v5.0.2 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cloudflare/circl v1.6.1 // indirect
	github.com/coreos/go-semver v0.3.0 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/cornelk/hashmap v1.0.8 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.5 // indirect
	github.com/cs3org/reva/v2 v2.27.2 // indirect
	github.com/cyphar/filepath-securejoin v0.2.5 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/fatih/color v1.14.1 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/gabriel-vasile/mimetype v1.4.8 // indirect
	github.com/go-asn1-ber/asn1-ber v1.5.8-0.20250403174932-29230038a667 // indirect
	github.com/go-git/gcfg v1.5.1-0.20230307220236-3a3c6141e376 // indirect
	github.com/go-git/go-billy/v5 v5.6.0 // indirect
	github.com/go-git/go-git/v5 v5.13.0 // indirect
	github.com/go-ini/ini v1.67.0 // indirect
	github.com/go-jose/go-jose/v4 v4.0.5 // indirect
	github.com/go-kit/log v0.2.1 // indirect
	github.com/go-logfmt/logfmt v0.5.1 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-micro/plugins/v4/logger/zerolog v1.2.0 // indirect
	github.com/go-micro/plugins/v4/registry/memory v1.2.0 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-openapi/errors v0.21.0 // indirect
	github.com/go-openapi/strfmt v0.22.0 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/pprof v0.0.0-20241210010833-40e02aabc2ad // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.27.1 // indirect
	github.com/hashicorp/yamux v0.1.1 // indirect
	github.com/imdario/mergo v0.3.15 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/kevinburke/ssh_config v1.2.0 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.10 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.15 // indirect
	github.com/mattn/go-tty v0.0.3 // indirect
	github.com/miekg/dns v1.1.57 // indirect
	github.com/minio/crc64nvme v1.0.1 // indirect
	github.com/minio/highwayhash v1.0.3 // indirect
	github.com/minio/md5-simd v1.1.2 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/nats-io/jwt/v2 v2.7.4 // indirect
	github.com/nats-io/nkeys v0.4.11 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/oklog/run v1.1.0 // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/oxtoacart/bpool v0.0.0-20190530202638-03653db5a59c // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/philhofer/fwd v1.1.3-0.20240916144458-20a13a1f6b7c // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
	github.com/pjbgf/sha1cd v0.3.0 // indirect
	github.com/pkg/term v1.2.0-beta.2 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.55.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/prometheus/statsd_exporter v0.22.8 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/rs/xid v1.6.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sergi/go-diff v1.3.2-0.20230802210424-5b0b94c5c0d3 // indirect
	github.com/shurcooL/httpfs v0.0.0-20190707220628-8d4bc4ba7749 // indirect
	github.com/shurcooL/vfsgen v0.0.0-20200824052919-0d455de96546 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/skeema/knownhosts v1.3.0 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/tinylib/msgp v1.3.0 // indirect
	github.com/tklauser/go-sysconf v0.3.14 // indirect
	github.com/tklauser/numcpus v0.8.0 // indirect
	github.com/urfave/cli/v2 v2.27.5 // indirect
	github.com/xanzy/ssh-agent v0.3.3 // indirect
	github.com/xrash/smetrics v0.0.0-20240521201337-686a1a2994c1 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.etcd.io/etcd/api/v3 v3.5.16 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.5.16 // indirect
	go.mongodb.org/mongo-driver v1.13.1 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.37.0 // indirect
	go.opentelemetry.io/otel/metric v1.37.0 // indirect
	go.opentelemetry.io/proto/otlp v1.7.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/multierr v1.9.0 // indirect
	go.uber.org/zap v1.23.0 // indirect
	golang.org/x/mod v0.25.0 // indirect
	golang.org/x/net v0.41.0 // indirect
	golang.org/x/time v0.11.0 // indirect
	golang.org/x/tools v0.33.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250603155806-513f23925822 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250603155806-513f23925822 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/studio-b12/gowebdav => github.com/kobergj/gowebdav v0.0.0-20250102091030-aa65266db202

replace github.com/go-micro/plugins/v4/store/nats-js-kv => github.com/kobergj/plugins/v4/store/nats-js-kv v0.0.0-20240807130109-f62bb67e8c90

replace go-micro.dev/v4 => github.com/kobergj/go-micro/v4 v4.0.0-20250610135441-d0b187215699

// exclude the v2 line of go-sqlite3 which was released accidentally and prevents pulling in newer versions of go-sqlite3
// see https://github.com/mattn/go-sqlite3/issues/965 for more details
exclude github.com/mattn/go-sqlite3 v2.0.3+incompatible
