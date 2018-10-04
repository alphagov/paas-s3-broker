package helpers

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/alphagov/paas-s3-broker/s3"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	awsS3 "github.com/aws/aws-sdk-go/service/s3"
	. "github.com/onsi/gomega"
)

const (
	fixturesPath = "../../fixtures/"
	testFileKey  = "file.txt"
)

func AssertBucketAccess(creds s3.BucketCredentials, bucketPrefix, bucketName, region string) {
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
