package helpers

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/alphagov/paas-s3-broker/s3"
	brokertesting "github.com/alphagov/paas-service-broker-base/testing"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	awsS3 "github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/codecommit"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	TestFileKey     = "test.txt"
	TestFileContent = "This is a test file"
)

func AssertBucketReadWriteAccess(creds s3.BucketCredentials, bucketPrefix, bucketName, region string) {
	s3Client := s3ClientFromCredentials(creds, region)
	bucketName = bucketPrefix + bucketName

	Eventually(func() error {
		return createS3Object(s3Client, TestFileContent, bucketName)
	}, 10*time.Second).ShouldNot(HaveOccurred())

	Eventually(func() error {
		return checkS3ObjectContent(s3Client, TestFileContent, bucketName)
	}, 10*time.Second).ShouldNot(HaveOccurred())

	Eventually(func() error {
		return checkListS3Bucket(s3Client, bucketName)
	}, 10*time.Second).ShouldNot(HaveOccurred())

	Eventually(func() error {
		return deleteS3Object(s3Client, bucketName)
	}, 10*time.Second).ShouldNot(HaveOccurred())
}

func AssertNoBucketAccess(creds s3.BucketCredentials, bucketPrefix, bucketName, region string) {
	s3Client := s3ClientFromCredentials(creds, region)
	bucketName = bucketPrefix + bucketName

	Consistently(func() error {
		return createS3Object(s3Client, TestFileContent, bucketName)
	}, 5*time.Second, 500*time.Millisecond).Should(HaveOccurred())

	Consistently(func() error {
		return checkListS3Bucket(s3Client, bucketName)
	}, 5*time.Second, 500*time.Millisecond).Should(HaveOccurred())

}

func AssertBucketReadOnlyAccess(creds s3.BucketCredentials, bucketPrefix, bucketName, region string) {
	s3Client := s3ClientFromCredentials(creds, region)
	bucketName = bucketPrefix + bucketName

	Eventually(func() error {
		return checkS3ObjectContent(s3Client, TestFileContent, bucketName)
	}, 10*time.Second).ShouldNot(HaveOccurred())

	Eventually(func() error {
		_, err := s3Client.PutObject(&awsS3.PutObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String("failed_to_create"),
			Body:   strings.NewReader(TestFileContent),
		})

		return err
	}, 10*time.Second).Should(HaveOccurred())
}

func AssertCodeCommitListReposAccess(creds s3.BucketCredentials, region string) {
	ccClient := codeCommitClientFromCredentials(creds, region)

	Eventually(func() error {
		_, err := ccClient.ListRepositories(&codecommit.ListRepositoriesInput{})

		return err
	}, 10*time.Second).ShouldNot(HaveOccurred())
}

func AssertCodeCommitListARTemplatesAccess(creds s3.BucketCredentials, region string) {
	ccClient := codeCommitClientFromCredentials(creds, region)

	Eventually(func() error {
		_, err := ccClient.ListApprovalRuleTemplates(&codecommit.ListApprovalRuleTemplatesInput{})

		return err
	}, 10*time.Second).ShouldNot(HaveOccurred())
}

func AssertNoCodeCommitListARTemplatesAccess(creds s3.BucketCredentials, region string) {
	ccClient := codeCommitClientFromCredentials(creds, region)

	Consistently(func() error {
		_, err := ccClient.ListApprovalRuleTemplates(&codecommit.ListApprovalRuleTemplatesInput{})

		return err
	}, 5*time.Second, 500*time.Millisecond).Should(MatchError(ContainSubstring("AccessDenied")))
}

func WriteTempFile(creds s3.BucketCredentials, bucketPrefix, bucketName, region string) {
	s3Client := s3ClientFromCredentials(creds, region)
	bucketName = bucketPrefix + bucketName

	Eventually(func() error {
		return createS3Object(s3Client, TestFileContent, bucketName)
	}, 10*time.Second).ShouldNot(HaveOccurred())
}

func DeleteTempFile(creds s3.BucketCredentials, bucketPrefix, bucketName, region string) {
	s3Client := s3ClientFromCredentials(creds, region)
	bucketName = bucketPrefix + bucketName

	Eventually(func() error {
		return deleteS3Object(s3Client, bucketName)
	}, 10*time.Second).ShouldNot(HaveOccurred())
}

func DeprovisionService(brokerTester brokertesting.BrokerTester, instanceID, serviceID, planID string) {
	By("Deprovisioning")
	res := brokerTester.Deprovision(instanceID, serviceID, planID, true)
	Expect(res.Code).To(Equal(http.StatusOK))
}

func Unbind(brokerTester brokertesting.BrokerTester, instanceID string, serviceID string, planID string, bindingID string) {
	By(fmt.Sprintf("Unbinding the %s binding", bindingID))

	// We use eventually here because, in test scenarios,
	// we can hit the get-bucket-policy endpoint before the
	// bucket policy has become consistent after an update.
	// we only want to stop when the broker can't find any
	// trace of the resources, which should result in a
	// 410
	Eventually(func() int {
		code := brokerTester.Unbind(instanceID, serviceID, planID, bindingID, true).Code
		Expect(code).To(BeNumerically("<", 500))
		return code
	}, 10*time.Second).Should(Equal(http.StatusGone))
}

func s3ClientFromCredentials(creds s3.BucketCredentials, region string) *awsS3.S3 {
	sess := session.Must(session.NewSession(&aws.Config{
		Region:      aws.String(region),
		Credentials: credentials.NewStaticCredentials(creds.AWSAccessKeyID, creds.AWSSecretAccessKey, ""),
	}))
	return awsS3.New(sess)
}

func codeCommitClientFromCredentials(creds s3.BucketCredentials, region string) *codecommit.CodeCommit {
	sess := session.Must(session.NewSession(&aws.Config{
		Region:      aws.String(region),
		Credentials: credentials.NewStaticCredentials(creds.AWSAccessKeyID, creds.AWSSecretAccessKey, ""),
	}))
	return codecommit.New(sess)
}

func createS3Object(s3Client *awsS3.S3, content string, bucketName string) error {
	_, err := s3Client.PutObject(&awsS3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(TestFileKey),
		Body:   strings.NewReader(content),
	})

	return err
}

func checkS3ObjectContent(s3Client *awsS3.S3, expectedContent string, bucketName string) error {
	getObjectOutput, err := s3Client.GetObject(&awsS3.GetObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(TestFileKey),
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
		Key:    aws.String(TestFileKey),
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
