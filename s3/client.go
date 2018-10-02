package s3

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

//go:generate counterfeiter -o fakes/fake_s3_client.go . Client
type Client interface {
	CreateBucket(name string) error
	DeleteBucket(name string) error
}

type S3Client struct {
	bucketPrefix string
	client       *s3.S3
}

func NewS3Client(bucketPrefix, region string) *S3Client {
	sess := session.Must(session.NewSession(&aws.Config{Region: aws.String(region)}))
	s3Client := s3.New(sess)
	return &S3Client{
		bucketPrefix: bucketPrefix,
		client:       s3Client,
	}
}

func (s *S3Client) CreateBucket(name string) error {
	_, err := s.client.CreateBucket(&s3.CreateBucketInput{
		Bucket: aws.String(s.bucketPrefix + name),
	})
	return err
}

func (s *S3Client) DeleteBucket(name string) error {
	_, err := s.client.DeleteBucket(&s3.DeleteBucketInput{
		Bucket: aws.String(s.bucketPrefix + name),
	})
	return err
}
