module github.com/opencloud-eu/reva/v2

go 1.24.1

require (
	bou.ke/monkey v1.0.2
	contrib.go.opencensus.io/exporter/prometheus v0.4.2
	github.com/BurntSushi/toml v1.5.0
	github.com/Masterminds/sprig v2.22.0+incompatible
	github.com/alexedwards/argon2id v1.0.0
	github.com/armon/go-radix v1.0.0
	github.com/beevik/etree v1.5.1
	github.com/bluele/gcache v0.0.2
	github.com/c-bata/go-prompt v0.2.6
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/ceph/go-ceph v0.35.0
	github.com/cheggaaa/pb/v3 v3.1.7
	github.com/coreos/go-oidc/v3 v3.15.0
	github.com/cs3org/cato v0.0.0-20200828125504-e418fc54dd5e
	github.com/cs3org/go-cs3apis v0.0.0-20250725064958-2d9caef4db2a
	github.com/dgraph-io/ristretto v0.2.0
	github.com/emvi/iso-639-1 v1.1.1
	github.com/eventials/go-tus v0.0.0-20220610120217-05d0564bb571
	github.com/gdexlab/go-render v1.0.1
	github.com/go-chi/chi/v5 v5.2.2
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
	github.com/go-sql-driver/mysql v1.9.3
	github.com/gofrs/flock v0.12.1
	github.com/golang-jwt/jwt/v5 v5.3.0
	github.com/golang/protobuf v1.5.4
	github.com/gomodule/redigo v1.9.2
	github.com/google/go-cmp v0.7.0
	github.com/google/go-github v17.0.0+incompatible
	github.com/google/renameio/v2 v2.0.0
	github.com/google/uuid v1.6.0
	github.com/grpc-ecosystem/go-grpc-middleware v1.4.0
	github.com/hashicorp/go-hclog v1.6.3
	github.com/hashicorp/go-plugin v1.7.0
	github.com/huandu/xstrings v1.5.0
	github.com/iancoleman/strcase v0.3.0
	github.com/jedib0t/go-pretty v4.3.0+incompatible
	github.com/jellydator/ttlcache/v2 v2.11.1
	github.com/juliangruber/go-intersect v1.1.0
	github.com/mattn/go-sqlite3 v1.14.32
	github.com/maxymania/go-system v0.0.0-20170110133659-647cc364bf0b
	github.com/mileusna/useragent v1.3.5
	github.com/minio/minio-go/v7 v7.0.95
	github.com/mitchellh/mapstructure v1.5.0
	github.com/nats-io/nats-server/v2 v2.11.6
	github.com/nats-io/nats.go v1.45.0
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/ginkgo/v2 v2.24.0
	github.com/onsi/gomega v1.38.0
	github.com/opencloud-eu/opencloud v1.1.0
	github.com/pablodz/inotifywaitgo v0.0.9
	github.com/pkg/errors v0.9.1
	github.com/pkg/xattr v0.4.12
	github.com/prometheus/alertmanager v0.28.1
	github.com/prometheus/client_golang v1.23.0
	github.com/rogpeppe/go-internal v1.14.1
	github.com/rs/cors v1.11.1
	github.com/rs/zerolog v1.34.0
	github.com/segmentio/kafka-go v0.4.49
	github.com/sercand/kuberesolver/v5 v5.1.1
	github.com/sethvargo/go-diceware v0.5.0
	github.com/sethvargo/go-password v0.3.1
	github.com/shamaton/msgpack/v2 v2.2.3
	github.com/shirou/gopsutil v3.21.11+incompatible
	github.com/stretchr/testify v1.10.0
	github.com/studio-b12/gowebdav v0.9.0
	github.com/test-go/testify v1.1.4
	github.com/thanhpk/randstr v1.0.6
	github.com/tus/tusd/v2 v2.8.0
	github.com/vmihailenco/msgpack/v5 v5.4.1
	github.com/wk8/go-ordered-map v1.0.0
	go-micro.dev/v4 v4.11.0
	go.etcd.io/etcd/client/v3 v3.6.4
	go.opencensus.io v0.24.0
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.62.0
	go.opentelemetry.io/otel v1.37.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.37.0
	go.opentelemetry.io/otel/sdk v1.37.0
	go.opentelemetry.io/otel/trace v1.37.0
	golang.org/x/crypto v0.41.0
	golang.org/x/exp v0.0.0-20250210185358-939b2ce775ac
	golang.org/x/oauth2 v0.30.0
	golang.org/x/sync v0.16.0
	golang.org/x/sys v0.35.0
	golang.org/x/term v0.34.0
	golang.org/x/text v0.28.0
	google.golang.org/genproto v0.0.0-20250303144028-a0af3efb3deb
	google.golang.org/grpc v1.75.0
	google.golang.org/protobuf v1.36.8
)

require (
	dario.cat/mergo v1.0.1 // indirect
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/Azure/go-ntlmssp v0.0.0-20221128193559-754e69321358 // indirect
	github.com/KimMachineGun/automemlimit v0.7.0 // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/Masterminds/semver/v3 v3.3.1 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/ProtonMail/go-crypto v1.1.5 // indirect
	github.com/VividCortex/ewma v1.2.0 // indirect
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bitly/go-simplejson v0.5.0 // indirect
	github.com/bmizerany/pat v0.0.0-20210406213842-e4b6760bdd6f // indirect
	github.com/cenkalti/backoff/v5 v5.0.2 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/chigopher/pathlib v0.19.1 // indirect
	github.com/cloudflare/circl v1.6.1 // indirect
	github.com/coreos/go-semver v0.3.1 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/cornelk/hashmap v1.0.8 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.6 // indirect
	github.com/cyphar/filepath-securejoin v0.3.6 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/davidbyttow/govips/v2 v2.16.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/fsnotify/fsnotify v1.8.0 // indirect
	github.com/gabriel-vasile/mimetype v1.4.8 // indirect
	github.com/go-asn1-ber/asn1-ber v1.5.8-0.20250403174932-29230038a667 // indirect
	github.com/go-git/gcfg v1.5.1-0.20230307220236-3a3c6141e376 // indirect
	github.com/go-git/go-billy/v5 v5.6.2 // indirect
	github.com/go-git/go-git/v5 v5.13.2 // indirect
	github.com/go-ini/ini v1.67.0 // indirect
	github.com/go-jose/go-jose/v4 v4.1.1 // indirect
	github.com/go-kit/log v0.2.1 // indirect
	github.com/go-logfmt/logfmt v0.5.1 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-micro/plugins/v4/logger/zerolog v1.2.0 // indirect
	github.com/go-micro/plugins/v4/registry/memory v1.2.0 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-openapi/errors v0.22.0 // indirect
	github.com/go-openapi/strfmt v0.23.0 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/go-tpm v0.9.5 // indirect
	github.com/google/pprof v0.0.0-20250403155104-27863c87afa6 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.27.1 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/hashicorp/yamux v0.1.2 // indirect
	github.com/imdario/mergo v0.3.15 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/jinzhu/copier v0.4.0 // indirect
	github.com/kevinburke/ssh_config v1.2.0 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.11 // indirect
	github.com/kovidgoyal/imaging v1.6.4 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/magiconair/properties v1.8.9 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/mattn/go-tty v0.0.3 // indirect
	github.com/miekg/dns v1.1.57 // indirect
	github.com/minio/crc64nvme v1.0.2 // indirect
	github.com/minio/highwayhash v1.0.3 // indirect
	github.com/minio/md5-simd v1.1.2 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
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
	github.com/pbnjay/memory v0.0.0-20210728143218-7b4eea64cf58 // indirect
	github.com/pelletier/go-toml/v2 v2.2.3 // indirect
	github.com/philhofer/fwd v1.2.0 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
	github.com/pjbgf/sha1cd v0.3.2 // indirect
	github.com/pkg/term v1.2.0-beta.2 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.65.0 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	github.com/prometheus/statsd_exporter v0.22.8 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/rs/xid v1.6.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sagikazarmark/locafero v0.7.0 // indirect
	github.com/sagikazarmark/slog-shim v0.1.0 // indirect
	github.com/sergi/go-diff v1.3.2-0.20230802210424-5b0b94c5c0d3 // indirect
	github.com/shurcooL/httpfs v0.0.0-20230704072500-f1e31cf0ba5c // indirect
	github.com/shurcooL/vfsgen v0.0.0-20230704071429-0000e147ea92 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/skeema/knownhosts v1.3.0 // indirect
	github.com/sourcegraph/conc v0.3.0 // indirect
	github.com/spf13/afero v1.12.0 // indirect
	github.com/spf13/cast v1.7.1 // indirect
	github.com/spf13/cobra v1.9.1 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	github.com/spf13/viper v1.19.0 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/tinylib/msgp v1.3.0 // indirect
	github.com/tklauser/go-sysconf v0.3.14 // indirect
	github.com/tklauser/numcpus v0.8.0 // indirect
	github.com/tus/tusd v1.10.0 // indirect
	github.com/urfave/cli/v2 v2.27.5 // indirect
	github.com/vektra/mockery/v2 v2.53.2 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/xanzy/ssh-agent v0.3.3 // indirect
	github.com/xrash/smetrics v0.0.0-20240521201337-686a1a2994c1 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.etcd.io/etcd/api/v3 v3.6.4 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.6.4 // indirect
	go.mongodb.org/mongo-driver v1.14.0 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/otel/exporters/jaeger v1.17.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.37.0 // indirect
	go.opentelemetry.io/otel/metric v1.37.0 // indirect
	go.opentelemetry.io/proto/otlp v1.7.0 // indirect
	go.uber.org/automaxprocs v1.6.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/image v0.24.0 // indirect
	golang.org/x/mod v0.27.0 // indirect
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/time v0.12.0 // indirect
	golang.org/x/tools v0.36.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250707201910-8d1bb00bc6a7 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250707201910-8d1bb00bc6a7 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/studio-b12/gowebdav => github.com/kobergj/gowebdav v0.0.0-20250102091030-aa65266db202

// exclude the v2 line of go-sqlite3 which was released accidentally and prevents pulling in newer versions of go-sqlite3
// see https://github.com/mattn/go-sqlite3/issues/965 for more details
exclude github.com/mattn/go-sqlite3 v2.0.3+incompatible

tool (
	github.com/vektra/mockery/v2
	golang.org/x/tools/cmd/goimports
)

replace github.com/go-micro/plugins/v4/store/nats-js-kv => github.com/opencloud-eu/go-micro-plugins/v4/store/nats-js-kv v0.0.0-20250512152754-23325793059a
