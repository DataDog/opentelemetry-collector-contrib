module github.com/open-telemetry/opentelemetry-collector-contrib/receiver/collectdreceiver

go 1.14

require (
	github.com/census-instrumentation/opencensus-proto v0.3.0
	github.com/stretchr/testify v1.7.0
	go.opencensus.io v0.22.6
	go.opentelemetry.io/collector v0.21.1-0.20210225192722-e6319ac4c6fc
	go.uber.org/zap v1.16.0
	google.golang.org/grpc/examples v0.0.0-20200728194956-1c32b02682df // indirect
	google.golang.org/protobuf v1.25.0
)
