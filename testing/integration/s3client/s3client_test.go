package s3client_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/alphagov/paas-s3-broker/s3"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	awsS3 "github.com/aws/aws-sdk-go/service/s3"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/satori/go.uuid"
)

const (
	region       = "eu-west-2"
	bucketPrefix = "gds-paas-s3-broker-testing-"
	fixturesPath = "../../fixtures/"
	testFileKey  = "file.txt"
)

var _ = Describe("S3client", func() {
	var (
		s3Client   *s3.S3Client
		bucketName string
		user1      string
		user2      string
	)

	BeforeEach(func() {
		s3Client = s3.NewS3Client(bucketPrefix, region)
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
		assertBucketAccess(user1Creds, bucketName)

		By("adding another user to the bucket policy")
		user2Creds, err := s3Client.AddUserToBucket(user2, bucketName)
		Expect(err).NotTo(HaveOccurred())

		By("using the returned credentials to prove access")
		assertBucketAccess(user2Creds, bucketName)

		By("preserving the first user's access")
		assertBucketAccess(user1Creds, bucketName)

		By("deleting a user")
		err = s3Client.RemoveUserFromBucket(user1, bucketName)
		Expect(err).NotTo(HaveOccurred())

		By("preserving the second user's access")
		assertBucketAccess(user2Creds, bucketName)

		By("deleting the final user")
		err = s3Client.RemoveUserFromBucket(user2, bucketName)
		Expect(err).NotTo(HaveOccurred())

		By("deleting the bucket")
		err = s3Client.DeleteBucket(bucketName)
		Expect(err).NotTo(HaveOccurred())
	})
})

func assertBucketAccess(creds s3.BucketCredentials, bucketName string) {
	sess := session.Must(session.NewSession(&aws.Config{
		Region:      aws.String(region),
		Credentials: credentials.NewStaticCredentials(creds.AWSAccessKeyID, creds.AWSSecretAccessKey, ""),
	}))
	userS3Client := awsS3.New(sess)
	file, err := os.Open(filepath.Join(fixturesPath, testFileKey))
	Expect(err).NotTo(HaveOccurred())
	defer file.Close()

	_, err = userS3Client.PutObject(&awsS3.PutObjectInput{
		Bucket: aws.String(bucketPrefix + bucketName),
		Key:    aws.String(testFileKey),
		Body:   file,
	})
	Expect(err).NotTo(HaveOccurred())

	getObjectOutput, err := userS3Client.GetObject(&awsS3.GetObjectInput{
		Bucket: aws.String(bucketPrefix + bucketName),
		Key:    aws.String(testFileKey),
	})
	Expect(err).NotTo(HaveOccurred())
	fileContents, err := ioutil.ReadAll(getObjectOutput.Body)
	Expect(err).NotTo(HaveOccurred())
	Expect(string(fileContents)).To(ContainSubstring("This is a file"))

	_, err = userS3Client.DeleteObject(&awsS3.DeleteObjectInput{
		Bucket: aws.String(bucketPrefix + bucketName),
		Key:    aws.String(testFileKey),
	})
	Expect(err).NotTo(HaveOccurred())
}
