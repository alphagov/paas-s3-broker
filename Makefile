.PHONY: unit
unit:
	ginkgo $(COMMAND) -r --skip-package=testing/integration $(PACKAGE)

.PHONY: test
test:
	go test -mod=vendor ./...

.PHONY: generate
generate:
	counterfeiter -o s3/fakes/fake_s3_api.go vendor/github.com/aws/aws-sdk-go/service/s3/s3iface/ S3API
	counterfeiter -o s3/fakes/fake_iam_api.go vendor/github.com/aws/aws-sdk-go/service/iam/iamiface/ IAMAPI
	go generate ./...
