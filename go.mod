module github.com/appscodelabs/gh-ci-webhook

go 1.18

require (
	github.com/go-chi/chi/v5 v5.0.8
	github.com/google/go-containerregistry v0.12.1
	github.com/google/go-github/v49 v49.1.0
	github.com/linode/linodego v1.9.3
	github.com/nats-io/nats.go v1.22.1
	github.com/pkg/errors v0.9.1
	github.com/spf13/cobra v1.6.0
	gocloud.dev v0.28.0
	golang.org/x/crypto v0.3.0
	golang.org/x/oauth2 v0.2.0
	gomodules.xyz/go-sh v0.1.0
	gomodules.xyz/logs v0.0.6
	gomodules.xyz/password-generator v0.2.9
	gomodules.xyz/pointer v0.1.0
	gomodules.xyz/signals v0.2.0
	gomodules.xyz/wait v0.2.0
	gomodules.xyz/x v0.0.14
	google.golang.org/grpc v1.51.0
	k8s.io/apimachinery v0.25.3
	k8s.io/klog/v2 v2.80.1
)

require (
	github.com/Masterminds/semver/v3 v3.1.1 // indirect
	github.com/codegangsta/inject v0.0.0-20150114235600-33e0aa1cb7c0 // indirect
	github.com/docker/cli v20.10.20+incompatible // indirect
	github.com/docker/distribution v2.8.1+incompatible // indirect
	github.com/docker/docker v20.10.21+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.7.0 // indirect
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/go-resty/resty/v2 v2.1.1-0.20191201195748-d7b97669fe48 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/googleapis/gax-go/v2 v2.7.0 // indirect
	github.com/inconshreveable/mousetrap v1.0.1 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/nats-io/nkeys v0.3.0 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/sirupsen/logrus v1.9.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	golang.org/x/net v0.4.1-0.20221210045730-e1ec361d0b39 // indirect
	golang.org/x/sys v0.3.0 // indirect
	golang.org/x/text v0.5.1-0.20221206182405-ec5565b1b747 // indirect
	golang.org/x/xerrors v0.0.0-20220907171357-04be3eba64a2 // indirect
	gomodules.xyz/clock v0.0.0-20200817085942-06523dba733f // indirect
	gomodules.xyz/flags v0.1.3 // indirect
	gomodules.xyz/sets v0.2.1 // indirect
	google.golang.org/api v0.103.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20221201204527-e3fa12d562f3 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/ini.v1 v1.66.6 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	k8s.io/utils v0.0.0-20221012122500-cfd413dd9e85 // indirect
	sigs.k8s.io/json v0.0.0-20220713155537-f223a00ba0e2 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
)

replace (
	go.opentelemetry.io/contrib => go.opentelemetry.io/contrib v0.20.0
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc => go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.20.0
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp => go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.20.0
	go.opentelemetry.io/otel => go.opentelemetry.io/otel v0.20.0
	go.opentelemetry.io/otel/exporters/otlp => go.opentelemetry.io/otel/exporters/otlp v0.20.0
	go.opentelemetry.io/otel/metric => go.opentelemetry.io/otel/metric v0.20.0
	go.opentelemetry.io/otel/oteltest => go.opentelemetry.io/otel/oteltest v0.20.0
	go.opentelemetry.io/otel/sdk => go.opentelemetry.io/otel/sdk v0.20.0
	go.opentelemetry.io/otel/sdk/export/metric => go.opentelemetry.io/otel/sdk/export/metric v0.20.0
	go.opentelemetry.io/otel/sdk/metric => go.opentelemetry.io/otel/sdk/metric v0.20.0
	go.opentelemetry.io/otel/trace => go.opentelemetry.io/otel/trace v0.20.0
	go.opentelemetry.io/proto/otlp => go.opentelemetry.io/proto/otlp v0.7.0
)

replace helm.sh/helm/v3 => github.com/kubepack/helm/v3 v3.9.1-0.20220603235400-7882cd0f0948

replace sigs.k8s.io/application => github.com/kmodules/application v0.8.4-0.20220603221517-a62565381f64
