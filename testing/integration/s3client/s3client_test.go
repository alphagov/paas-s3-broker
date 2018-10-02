package s3client_test

import (
	"github.com/alphagov/paas-s3-broker/s3"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/satori/go.uuid"
)

var _ = Describe("S3client", func() {
	var (
		s3Client   *s3.S3Client
		bucketName string
	)

	BeforeEach(func() {
		s3Client = s3.NewS3Client("gds-paas-s3-broker-testing-", "eu-west-2")
		bucketName = uuid.NewV4().String()
	})

	It("can manage the S3 bucket lifecycle", func() {
		By("creating a bucket")
		err := s3Client.CreateBucket(bucketName)
		Expect(err).NotTo(HaveOccurred())

		By("deleting the bucket")
		err = s3Client.DeleteBucket(bucketName)
		Expect(err).NotTo(HaveOccurred())
	})
})
