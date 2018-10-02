package s3client_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestS3client(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "S3client Suite")
}
