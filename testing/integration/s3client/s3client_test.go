package s3client_test

import (
	"github.com/alphagov/paas-s3-broker/s3"
	"github.com/alphagov/paas-s3-broker/testing/integration/helpers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/satori/go.uuid"
)

const (
	region       = "eu-west-2"
	bucketPrefix = "test-paas-s3-broker-"
	iamUserPath  = "/test-paas-s3-broker/"
)

var _ = Describe("S3client", func() {
	var (
		s3Client   *s3.S3Client
		bucketName string
		user1      string
		user2      string
	)

	BeforeEach(func() {
		s3Client = s3.NewS3Client(bucketPrefix, iamUserPath, region)
		bucketName = uuid.NewV4().String()
		user1 = uuid.NewV4().String()
		user2 = uuid.NewV4().String()
	})

	It("can manage the S3 bucket lifecycle", func() {
		By("creating a bucket")
		err := s3Client.CreateBucket(bucketName)
		Expect(err).NotTo(HaveOccurred())

		By("adding a user to the bucket policy")
		user1Creds, err := s3Client.AddUserToBucket(user1, bucketName)
		Expect(err).NotTo(HaveOccurred())

		By("using the returned credentials to prove access")
		helpers.AssertBucketAccess(user1Creds, bucketPrefix, bucketName, region)

		By("adding another user to the bucket policy")
		user2Creds, err := s3Client.AddUserToBucket(user2, bucketName)
		Expect(err).NotTo(HaveOccurred())

		By("using the returned credentials to prove access")
		helpers.AssertBucketAccess(user2Creds, bucketPrefix, bucketName, region)

		By("preserving the first user's access")
		helpers.AssertBucketAccess(user1Creds, bucketPrefix, bucketName, region)

		By("deleting a user")
		err = s3Client.RemoveUserFromBucket(user1, bucketName)
		Expect(err).NotTo(HaveOccurred())

		By("preserving the second user's access")
		helpers.AssertBucketAccess(user2Creds, bucketPrefix, bucketName, region)

		By("deleting the final user")
		err = s3Client.RemoveUserFromBucket(user2, bucketName)
		Expect(err).NotTo(HaveOccurred())

		By("deleting the bucket")
		err = s3Client.DeleteBucket(bucketName)
		Expect(err).NotTo(HaveOccurred())
	})
})
