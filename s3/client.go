package s3

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/iam/iamiface"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"strings"
)

//go:generate counterfeiter -o fakes/fake_s3_client.go . Client
type Client interface {
	CreateBucket(name string) error
	DeleteBucket(name string) error
	AddUserToBucket(username, bucketName string) (BucketCredentials, error)
	RemoveUserFromBucket(username, bucketName string) error
}

type BucketCredentials struct {
	BucketName         string `json:"bucket_name"`
	AWSAccessKeyID     string `json:"aws_access_key_id"`
	AWSSecretAccessKey string `json:"aws_secret_access_key"`
}

type S3Client struct {
	bucketPrefix string
	Timeout      time.Duration
	S3           s3iface.S3API
	IAM          iamiface.IAMAPI
}

func NewS3Client(bucketPrefix, region string) *S3Client {
	sess := session.Must(session.NewSession(&aws.Config{Region: aws.String(region)}))
	s3Client := s3.New(sess)
	iamClient := iam.New(sess)
	return &S3Client{
		bucketPrefix: bucketPrefix,
		Timeout:      30 * time.Second,
		S3:           s3Client,
		IAM:          iamClient,
	}
}

func (s *S3Client) CreateBucket(name string) error {
	_, err := s.S3.CreateBucket(&s3.CreateBucketInput{
		Bucket: aws.String(s.buildBucketName(name)),
	})
	return err
}

func (s *S3Client) DeleteBucket(name string) error {
	_, err := s.S3.DeleteBucket(&s3.DeleteBucketInput{
		Bucket: aws.String(s.buildBucketName(name)),
	})
	return err
}

func (s *S3Client) AddUserToBucket(username, bucketName string) (BucketCredentials, error) {
	fullBucketName := s.buildBucketName(bucketName)
	createUserOutput, err := s.IAM.CreateUser(&iam.CreateUserInput{
		Path:     aws.String("/s3-broker/"),
		UserName: aws.String(username),
	})
	if err != nil {
		return BucketCredentials{}, err
	}

	createAccessKeyOutput, err := s.IAM.CreateAccessKey(&iam.CreateAccessKeyInput{
		UserName: aws.String(username),
	})
	if err != nil {
		s.deleteUserWithoutError(username)
		return BucketCredentials{}, err
	}

	getBucketPolicyOutput, err := s.S3.GetBucketPolicy(&s3.GetBucketPolicyInput{
		Bucket: aws.String(fullBucketName),
	})
	currentBucketPolicy := ""
	if err != nil {
		if !strings.Contains(err.Error(), "NoSuchBucketPolicy: The bucket policy does not exist") {
			s.deleteUserWithoutError(username)
			return BucketCredentials{}, err
		}
	} else {
		currentBucketPolicy = *getBucketPolicyOutput.Policy
	}

	updatedBucketPolicy, err := s.AddUserToBucketPolicy(*createUserOutput.User.Arn, fullBucketName, currentBucketPolicy)
	if err != nil {
		s.deleteUserWithoutError(username)
		return BucketCredentials{}, err
	}

	updatedPolicyJSON, err := json.Marshal(updatedBucketPolicy)
	if err != nil {
		s.deleteUserWithoutError(username)
		return BucketCredentials{}, err
	}

	err = s.putBucketPolicyWithTimeout(fullBucketName, string(updatedPolicyJSON))
	if err != nil {
		s.deleteUserWithoutError(username)
		return BucketCredentials{}, err
	}

	return BucketCredentials{
		BucketName:         fullBucketName,
		AWSAccessKeyID:     *createAccessKeyOutput.AccessKey.AccessKeyId,
		AWSSecretAccessKey: *createAccessKeyOutput.AccessKey.SecretAccessKey,
	}, nil
}

func (s *S3Client) putBucketPolicyWithTimeout(fullBucketName, updatedPolicyJSON string) error {
	var apiErr error
	timeoutChannel := make(chan bool)
	go func() {
		time.Sleep(s.Timeout)
		timeoutChannel <- true
	}()
	for {
		select {
		case <-timeoutChannel:
			return apiErr
		default:
			_, apiErr = s.S3.PutBucketPolicy(&s3.PutBucketPolicyInput{
				Bucket: aws.String(fullBucketName),
				Policy: aws.String(updatedPolicyJSON),
			})
			if apiErr == nil {
				return apiErr
			}
			time.Sleep(2 * time.Second)
		}
	}
}

func (s *S3Client) deleteUserWithoutError(username string) {
	_ = s.deleteUser(username)
}

func (s *S3Client) deleteUser(username string) error {
	keys, err := s.IAM.ListAccessKeys(&iam.ListAccessKeysInput{
		UserName: aws.String(username),
	})
	if err != nil {
		return err
	}
	if keys != nil {
		for _, k := range keys.AccessKeyMetadata {
			_, err := s.IAM.DeleteAccessKey(&iam.DeleteAccessKeyInput{
				UserName:    aws.String(username),
				AccessKeyId: k.AccessKeyId,
			})
			if err != nil {
				return err
			}
		}
	}
	_, err = s.IAM.DeleteUser(&iam.DeleteUserInput{
		UserName: aws.String(username),
	})
	return err
}

func (s *S3Client) buildBucketArn(fullBucketName string) string {
	return fmt.Sprintf("arn:aws:s3:::%s/*", fullBucketName)
}

func (s *S3Client) buildBucketName(bucketName string) string {
	return fmt.Sprintf("%s%s", s.bucketPrefix, bucketName)
}

func (s *S3Client) AddUserToBucketPolicy(userArn, fullBucketName, policyDocument string) (PolicyDocument, error) {
	updatedPolicy := BucketPolicyTemplate
	updatedPolicy.Statement[0].Resource = s.buildBucketArn(fullBucketName)

	if policyDocument == "" {
		updatedPolicy.Statement[0].Principal = Principal{
			AWS: userArn,
		}
		return updatedPolicy, nil
	}

	err := json.Unmarshal([]byte(policyDocument), &updatedPolicy)
	if err != nil {
		return updatedPolicy, err
	}

	principal := updatedPolicy.Statement[0].Principal.AWS
	switch p := principal.(type) {
	case string:
		updatedPolicy.Statement[0].Principal.AWS = []string{p, userArn}
	case []interface{}:
		principals := []string{}
		for _, i := range p {
			principals = append(principals, i.(string))
		}
		principals = append(
			principals,
			userArn,
		)
		updatedPolicy.Statement[0].Principal.AWS = principals
	}

	return updatedPolicy, nil
}

func (s *S3Client) RemoveUserFromBucket(username, bucketName string) error {
	fullBucketName := s.buildBucketName(bucketName)
	getBucketPolicyOutput, err := s.S3.GetBucketPolicy(&s3.GetBucketPolicyInput{
		Bucket: aws.String(fullBucketName),
	})
	if err != nil {
		return err
	}

	_, err = s.RemoveUserFromBucketPolicy(username, fullBucketName, *getBucketPolicyOutput.Policy)
	if err != nil {
		return err
	}

	err = s.deleteUser(username)

	return err
}

func (s *S3Client) RemoveUserFromBucketPolicy(username, fullBucketName, currentBucketPolicy string) (PolicyDocument, error) {
	updatedPolicy := BucketPolicyTemplate
	updatedPolicy.Statement[0].Resource = s.buildBucketArn(fullBucketName)

	err := json.Unmarshal([]byte(currentBucketPolicy), &updatedPolicy)
	if err != nil {
		return updatedPolicy, err
	}

	principal := updatedPolicy.Statement[0].Principal.AWS
	switch p := principal.(type) {
	// string means there was only 1 principal - policy can be deleted.
	case string:
		_, err = s.S3.DeleteBucketPolicy(&s3.DeleteBucketPolicyInput{
			Bucket: aws.String(fullBucketName),
		})
		if err != nil {
			return updatedPolicy, err
		}
	case []interface{}:
		principals := []string{}
		for _, i := range p {
			if !s.arnIsForUser(i.(string), username) {
				principals = append(principals, i.(string))
			}
		}
		updatedPolicy.Statement[0].Principal.AWS = principals
		updatedPolicyJSON, err := json.Marshal(updatedPolicy)
		if err != nil {
			return updatedPolicy, err
		}

		err = s.putBucketPolicyWithTimeout(fullBucketName, string(updatedPolicyJSON))
		if err != nil {
			return updatedPolicy, err
		}
	}

	return updatedPolicy, nil
}

func (s *S3Client) arnIsForUser(arn, username string) bool {
	return strings.HasSuffix(arn, username)
}

type PolicyDocument struct {
	Version   string      `json:"Version,omitempty"`
	Statement []Statement `json:"Statement,omitempty"`
}

type Statement struct {
	Effect    string    `json:"Effect,omitempty"`
	Action    []string  `json:"Action,omitempty"`
	Resource  string    `json:"Resource,omitempty"`
	Principal Principal `json:"Principal,omitempty"`
}

type Principal struct {
	AWS interface{} `json:"AWS,omitempty"`
}

var BucketPolicyTemplate = PolicyDocument{
	Version: "2012-10-17",
	Statement: []Statement{
		{
			Effect: "Allow",
			Action: []string{"s3:DeleteObject", "s3:GetObject", "s3:PutObject"},
		},
	},
}
