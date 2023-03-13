module github.com/cs3org/reva/v2

go 1.19

require (
	bou.ke/monkey v1.0.2
	contrib.go.opencensus.io/exporter/prometheus v0.4.2
	github.com/BurntSushi/toml v1.2.1
	github.com/CiscoM31/godata v1.0.5
	github.com/Masterminds/sprig v2.22.0+incompatible
	github.com/aws/aws-sdk-go v1.44.122
	github.com/beevik/etree v1.1.0
	github.com/bluele/gcache v0.0.2
	github.com/c-bata/go-prompt v0.2.6
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/ceph/go-ceph v0.18.0
	github.com/cheggaaa/pb v1.0.29
	github.com/coreos/go-oidc v2.2.1+incompatible
	github.com/cs3org/cato v0.0.0-20200828125504-e418fc54dd5e
	github.com/cs3org/go-cs3apis v0.0.0-20221012090518-ef2996678965
	github.com/cubewise-code/go-mime v0.0.0-20200519001935-8c5762b177d8
	github.com/dgraph-io/ristretto v0.1.1
	github.com/emvi/iso-639-1 v1.0.1
	github.com/eventials/go-tus v0.0.0-20220610120217-05d0564bb571
	github.com/gdexlab/go-render v1.0.1
	github.com/go-chi/chi/v5 v5.0.7
	github.com/go-ldap/ldap/v3 v3.4.4
	github.com/go-micro/plugins/v4/events/natsjs v1.1.0
	github.com/go-micro/plugins/v4/server/http v1.1.1
	github.com/go-micro/plugins/v4/store/nats-js v1.1.0
	github.com/go-micro/plugins/v4/store/redis v1.2.0
	github.com/go-redis/redis/v8 v8.11.5
	github.com/go-sql-driver/mysql v1.6.0
	github.com/gofrs/flock v0.8.1
	github.com/golang-jwt/jwt v3.2.2+incompatible
	github.com/golang/protobuf v1.5.2
	github.com/gomodule/redigo v1.8.9
	github.com/google/go-cmp v0.5.9
	github.com/google/go-github v17.0.0+incompatible
	github.com/google/uuid v1.3.0
	github.com/grpc-ecosystem/go-grpc-middleware v1.3.0
	github.com/hashicorp/go-hclog v1.3.1
	github.com/hashicorp/go-plugin v1.4.5
	github.com/iancoleman/strcase v0.2.0
	github.com/jedib0t/go-pretty v4.3.0+incompatible
	github.com/jellydator/ttlcache/v2 v2.11.1
	github.com/juliangruber/go-intersect v1.1.0
	github.com/mattn/go-sqlite3 v2.0.3+incompatible
	github.com/maxymania/go-system v0.0.0-20170110133659-647cc364bf0b
	github.com/mileusna/useragent v1.2.1
	github.com/minio/minio-go/v7 v7.0.42
	github.com/mitchellh/mapstructure v1.5.0
	github.com/nats-io/nats-server/v2 v2.9.3
	github.com/nats-io/nats.go v1.18.0
	github.com/onsi/ginkgo/v2 v2.7.0
	github.com/onsi/gomega v1.25.0
	github.com/owncloud/ocis/v2 v2.0.0-beta2.0.20220929090834-3b95dcc545dc
	github.com/pkg/errors v0.9.1
	github.com/pkg/xattr v0.4.9
	github.com/prometheus/alertmanager v0.24.0
	github.com/prometheus/client_golang v1.13.0
	github.com/rogpeppe/go-internal v1.6.1
	github.com/rs/cors v1.8.2
	github.com/rs/zerolog v1.28.0
	github.com/sciencemesh/meshdirectory-web v1.0.4
	github.com/sethvargo/go-password v0.2.0
	github.com/shamaton/msgpack/v2 v2.1.1
	github.com/stretchr/testify v1.8.1
	github.com/studio-b12/gowebdav v0.0.0-20221015232716-17255f2e7423
	github.com/test-go/testify v1.1.4
	github.com/thanhpk/randstr v1.0.4
	github.com/tus/tusd v1.10.0
	github.com/wk8/go-ordered-map v1.0.0
	go-micro.dev/v4 v4.9.0
	go.opencensus.io v0.23.0
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.36.4
	go.opentelemetry.io/otel v1.11.1
	go.opentelemetry.io/otel/exporters/jaeger v1.11.1
	go.opentelemetry.io/otel/sdk v1.11.1
	go.opentelemetry.io/otel/trace v1.11.1
	golang.org/x/crypto v0.1.0
	golang.org/x/oauth2 v0.1.0
	golang.org/x/sync v0.1.0
	golang.org/x/sys v0.4.0
	golang.org/x/term v0.4.0
	golang.org/x/text v0.6.0
	google.golang.org/genproto v0.0.0-20221024183307-1bc688fe9f3e
	google.golang.org/grpc v1.50.1
	google.golang.org/protobuf v1.28.1
	gotest.tools v2.2.0+incompatible
)

require (
	github.com/Azure/go-ntlmssp v0.0.0-20220621081337-cb9428e4ac1e // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/Microsoft/go-winio v0.5.2 // indirect
	github.com/ProtonMail/go-crypto v0.0.0-20220824120805-4b6e5c587895 // indirect
	github.com/acomagu/bufpipe v1.0.3 // indirect
	github.com/armon/go-metrics v0.4.1 // indirect
	github.com/asaskevich/govalidator v0.0.0-20210307081110-f21760c49a8d // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bitly/go-simplejson v0.5.0 // indirect
	github.com/bmizerany/pat v0.0.0-20210406213842-e4b6760bdd6f // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/cloudflare/circl v1.2.0 // indirect
	github.com/coreos/go-semver v0.3.0 // indirect
	github.com/coreos/go-systemd/v22 v22.4.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/dustin/go-humanize v1.0.0 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/fatih/color v1.13.0 // indirect
	github.com/fsnotify/fsnotify v1.5.4 // indirect
	github.com/go-asn1-ber/asn1-ber v1.5.4 // indirect
	github.com/go-git/gcfg v1.5.0 // indirect
	github.com/go-git/go-billy/v5 v5.3.1 // indirect
	github.com/go-git/go-git/v5 v5.4.2 // indirect
	github.com/go-kit/log v0.2.1 // indirect
	github.com/go-logfmt/logfmt v0.5.1 // indirect
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-micro/plugins/v4/registry/consul v1.1.0 // indirect
	github.com/go-micro/plugins/v4/registry/etcd v1.1.0 // indirect
	github.com/go-micro/plugins/v4/registry/kubernetes v1.1.0 // indirect
	github.com/go-micro/plugins/v4/registry/mdns v1.1.0 // indirect
	github.com/go-micro/plugins/v4/registry/memory v1.1.0 // indirect
	github.com/go-micro/plugins/v4/registry/nats v1.1.0 // indirect
	github.com/go-openapi/errors v0.20.2 // indirect
	github.com/go-openapi/strfmt v0.21.2 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/glog v1.0.0 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/hashicorp/consul/api v1.15.2 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/hashicorp/serf v0.10.0 // indirect
	github.com/hashicorp/yamux v0.0.0-20211028200310-0bc27b27de87 // indirect
	github.com/huandu/xstrings v1.3.2 // indirect
	github.com/imdario/mergo v0.3.13 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kevinburke/ssh_config v1.2.0 // indirect
	github.com/klauspost/compress v1.15.11 // indirect
	github.com/klauspost/cpuid/v2 v2.1.0 // indirect
	github.com/kr/pretty v0.3.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.16 // indirect
	github.com/mattn/go-runewidth v0.0.13 // indirect
	github.com/mattn/go-tty v0.0.3 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.1 // indirect
	github.com/miekg/dns v1.1.50 // indirect
	github.com/minio/highwayhash v1.0.2 // indirect
	github.com/minio/md5-simd v1.1.2 // indirect
	github.com/minio/sha256-simd v1.0.0 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/go-testing-interface v1.14.1 // indirect
	github.com/mitchellh/hashstructure v1.1.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/nats-io/jwt/v2 v2.3.0 // indirect
	github.com/nats-io/nkeys v0.3.0 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/oklog/run v1.1.0 // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/oxtoacart/bpool v0.0.0-20190530202638-03653db5a59c // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pkg/term v1.2.0-beta.2 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/pquerna/cachecontrol v0.1.0 // indirect
	github.com/prometheus/client_model v0.3.0 // indirect
	github.com/prometheus/common v0.37.0 // indirect
	github.com/prometheus/procfs v0.8.0 // indirect
	github.com/prometheus/statsd_exporter v0.22.8 // indirect
	github.com/rivo/uniseg v0.4.2 // indirect
	github.com/rs/xid v1.4.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sergi/go-diff v1.2.0 // indirect
	github.com/shurcooL/httpfs v0.0.0-20190707220628-8d4bc4ba7749 // indirect
	github.com/shurcooL/vfsgen v0.0.0-20200824052919-0d455de96546 // indirect
	github.com/sirupsen/logrus v1.9.0 // indirect
	github.com/stretchr/objx v0.5.0 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/urfave/cli/v2 v2.20.3 // indirect
	github.com/xanzy/ssh-agent v0.3.2 // indirect
	github.com/xrash/smetrics v0.0.0-20201216005158-039620a65673 // indirect
	go.etcd.io/etcd/api/v3 v3.5.5 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.5.5 // indirect
	go.etcd.io/etcd/client/v3 v3.5.5 // indirect
	go.mongodb.org/mongo-driver v1.10.3 // indirect
	go.uber.org/atomic v1.10.0 // indirect
	go.uber.org/multierr v1.8.0 // indirect
	go.uber.org/zap v1.23.0 // indirect
	golang.org/x/mod v0.7.0 // indirect
	golang.org/x/net v0.5.0 // indirect
	golang.org/x/time v0.1.0 // indirect
	golang.org/x/tools v0.4.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	gopkg.in/ini.v1 v1.66.6 // indirect
	gopkg.in/square/go-jose.v2 v2.6.0 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/cs3org/go-cs3apis => github.com/c0rby/go-cs3apis v0.0.0-20230110100311-5b424f1baa35
