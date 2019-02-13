package helpers

import (
	"errors"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/alphagov/paas-s3-broker/s3"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	awsS3 "github.com/aws/aws-sdk-go/service/s3"
	. "github.com/onsi/gomega"
)

const (
	testFileKey = "test.txt"
)

func AssertBucketAccess(creds s3.BucketCredentials, bucketPrefix, bucketName, region string) {
	sess := session.Must(session.NewSession(&aws.Config{
		Region:      aws.String(region),
		Credentials: credentials.NewStaticCredentials(creds.AWSAccessKeyID, creds.AWSSecretAccessKey, ""),
	}))
	s3Client := awsS3.New(sess)

	testFileContent := "This is a test file"
	bucketName = bucketPrefix + bucketName

	Eventually(func() error {
		return createS3Object(s3Client, testFileContent, bucketName)
	}, 10*time.Second).ShouldNot(HaveOccurred())

	Eventually(func() error {
		return checkS3ObjectContent(s3Client, testFileContent, bucketName)
	}, 10*time.Second).ShouldNot(HaveOccurred())

	Eventually(func() error {
		return checkListS3Bucket(s3Client, bucketName)
	}, 10*time.Second).ShouldNot(HaveOccurred())

	Eventually(func() error {
		return deleteS3Object(s3Client, bucketName)
	}, 10*time.Second).ShouldNot(HaveOccurred())
}

func createS3Object(s3Client *awsS3.S3, content string, bucketName string) error {
	_, err := s3Client.PutObject(&awsS3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(testFileKey),
		Body:   strings.NewReader(content),
	})

	return err
}

func checkS3ObjectContent(s3Client *awsS3.S3, expectedContent string, bucketName string) error {
	getObjectOutput, err := s3Client.GetObject(&awsS3.GetObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(testFileKey),
	})
	if err != nil {
		return err
	}

	fileContents, err := ioutil.ReadAll(getObjectOutput.Body)
	if err != nil {
		return err
	}
	defer getObjectOutput.Body.Close()

	if string(fileContents) != expectedContent {
		return fmt.Errorf("content mismatch, got %q", string(fileContents))
	}

	return nil
}

func deleteS3Object(s3Client *awsS3.S3, bucketName string) error {
	_, err := s3Client.DeleteObject(&awsS3.DeleteObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(testFileKey),
	})

	return err
}

func checkListS3Bucket(s3Client *awsS3.S3, bucketName string) error {
	listBucketOutput, err := s3Client.ListObjects(&awsS3.ListObjectsInput{
		Bucket: aws.String(bucketName),
	})

	if err != nil {
		return err
	}
	if len(listBucketOutput.Contents) != 1 {
		return errors.New("Bucket should not be empty")
	}
	return nil
}
