.PHONY: unit
unit:
	go run github.com/onsi/ginkgo/v2/ginkgo $(COMMAND) -r --skip-package=testing/integration $(PACKAGE)

.PHONY: test
test:
	go test -mod=vendor ./...

.PHONY: generate
generate:
	go run github.com/maxbrunsfeld/counterfeiter/v6 -o s3/fakes/fake_s3_api.go vendor/github.com/aws/aws-sdk-go/service/s3/s3iface/ S3API
	go run github.com/maxbrunsfeld/counterfeiter/v6 -o s3/fakes/fake_iam_api.go vendor/github.com/aws/aws-sdk-go/service/iam/iamiface/ IAMAPI
	go generate ./...

.PHONY: build_amd64
build_amd64:
	mkdir -p amd64
	GOOS=linux GOARCH=amd64 go build -o amd64/s3-broker

.PHONY: bosh_scp
bosh_scp: build_amd64
	./scripts/bosh-scp.sh

.PHONY: run_broker_tls
run_broker_tls:
	./scripts/run-broker-tls.sh
