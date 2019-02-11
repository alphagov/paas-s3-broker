package s3

import (
	"encoding/json"
	"fmt"
	"time"

	"strings"

	"github.com/alphagov/paas-go/provider"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/iam/iamiface"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
)

//go:generate counterfeiter -o fakes/fake_s3_client.go . Client
type Client interface {
	CreateBucket(provisionData provider.ProvisionData, deployEnv string) error
	DeleteBucket(name string) error
	AddUserToBucket(username, bucketName, awsRegion string) (BucketCredentials, error)
	RemoveUserFromBucket(username, bucketName string) error
}

type BucketCredentials struct {
	BucketName         string `json:"bucket_name"`
	AWSAccessKeyID     string `json:"aws_access_key_id"`
	AWSSecretAccessKey string `json:"aws_secret_access_key"`
	AWSRegion          string `json:"aws_region"`
}

type S3Client struct {
	bucketPrefix string
	iamUserPath  string
	Timeout      time.Duration
	S3           s3iface.S3API
	IAM          iamiface.IAMAPI
}

func NewS3Client(bucketPrefix, iamUserPath, region string) *S3Client {
	sess := session.Must(session.NewSession(&aws.Config{Region: aws.String(region)}))
	s3Client := s3.New(sess)
	iamClient := iam.New(sess)
	return &S3Client{
		bucketPrefix: bucketPrefix,
		iamUserPath:  fmt.Sprintf("/%s/", strings.Trim(iamUserPath, "/")),
		Timeout:      30 * time.Second,
		S3:           s3Client,
		IAM:          iamClient,
	}
}

func (s *S3Client) CreateBucket(provisionData provider.ProvisionData, deployEnvironment string) error {
	_, err := s.S3.CreateBucket(&s3.CreateBucketInput{
		Bucket: aws.String(s.buildBucketName(provisionData.InstanceID)),
	})
	if err != nil {
		return err
	}
	_, err = s.tagBucket(provisionData.InstanceID, []*s3.Tag{
		{
			Key:   aws.String("service_instance_guid"),
			Value: aws.String(provisionData.InstanceID),
		},
		{
			Key:   aws.String("org_guid"),
			Value: aws.String(provisionData.Details.OrganizationGUID),
		},
		{
			Key:   aws.String("space_guid"),
			Value: aws.String(provisionData.Details.SpaceGUID),
		},
		{
			Key:   aws.String("created_by"),
			Value: aws.String("paas-s3-broker"),
		},
		{
			Key:   aws.String("plan_guid"),
			Value: aws.String(provisionData.Plan.ID),
		},
		{
			Key:   aws.String("deploy_env"),
			Value: aws.String(deployEnvironment),
		},
	})
	if err != nil {
		deleteErr := s.DeleteBucket(provisionData.InstanceID)
		if deleteErr != nil {
			return fmt.Errorf(
				"error while tagging S3 Bucket %s: %v.\nadditional error while deleting %s: %v",
				provisionData.InstanceID, err, provisionData.InstanceID, deleteErr,
			)
		}
		return fmt.Errorf("error while tagging S3 Bucket %s: %v. Bucket has been deleted", provisionData.InstanceID, err)
	}
	return err
}

func (s *S3Client) DeleteBucket(name string) error {
	_, err := s.S3.DeleteBucket(&s3.DeleteBucketInput{
		Bucket: aws.String(s.buildBucketName(name)),
	})
	return err
}

func (s *S3Client) AddUserToBucket(username, bucketName, awsRegion string) (BucketCredentials, error) {
	fullBucketName := s.buildBucketName(bucketName)
	createUserOutput, err := s.IAM.CreateUser(&iam.CreateUserInput{
		Path:     aws.String(s.iamUserPath),
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
		AWSRegion:          awsRegion,
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

func (s *S3Client) tagBucket(instanceID string, tags []*s3.Tag) (output *s3.PutBucketTaggingOutput, err error) {
	createTagsInput := s3.PutBucketTaggingInput{
		Bucket:  aws.String(s.buildBucketName(instanceID)),
		Tagging: &s3.Tagging{TagSet: tags},
	}
	result, err := s.S3.PutBucketTagging(&createTagsInput)
	return result, err
}

func (s *S3Client) buildBucketArn(fullBucketName string) string {
	return fmt.Sprintf("arn:aws:s3:::%s/*", fullBucketName)
}

func (s *S3Client) buildBareBucketArn(fullBucketName string) string {
	return fmt.Sprintf("arn:aws:s3:::%s", fullBucketName)
}

func (s *S3Client) buildBucketName(bucketName string) string {
	return fmt.Sprintf("%s%s", s.bucketPrefix, bucketName)
}

func (s *S3Client) AddUserToBucketPolicy(userArn, fullBucketName, policyDocument string) (PolicyDocument, error) {
	updatedPolicy := BucketPolicyTemplate
	updatedPolicy.Statement[0].Resource = s.buildBucketArn(fullBucketName)
	updatedPolicy.Statement[1].Resource = s.buildBareBucketArn(fullBucketName)

	if policyDocument == "" {
		for policyStatementIndex, _ := range updatedPolicy.Statement {
			updatedPolicy.Statement[policyStatementIndex].Principal = Principal{
				AWS: userArn,
			}
		}
		return updatedPolicy, nil
	}

	err := json.Unmarshal([]byte(policyDocument), &updatedPolicy)
	if err != nil {
		return updatedPolicy, err
	}
	for policyStatementIndex := range updatedPolicy.Statement {
		principal := updatedPolicy.Statement[policyStatementIndex].Principal.AWS
		switch p := principal.(type) {
		case string:
			updatedPolicy.Statement[policyStatementIndex].Principal.AWS = []string{p, userArn}
		case []interface{}:
			principals := []string{}
			for _, i := range p {
				principals = append(principals, i.(string))
			}
			principals = append(
				principals,
				userArn,
			)
			updatedPolicy.Statement[policyStatementIndex].Principal.AWS = principals
		}
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
	for policyStatementIndex := range updatedPolicy.Statement {
		principal := updatedPolicy.Statement[policyStatementIndex].Principal.AWS
		switch p := principal.(type) {
		case string:
			updatedPolicy.Statement[policyStatementIndex].Principal.AWS = nil
		case []interface{}:
			principals := []string{}
			for _, i := range p {
				if !s.arnIsForUser(i.(string), username) {
					principals = append(principals, i.(string))
				}
			}
			updatedPolicy.Statement[policyStatementIndex].Principal.AWS = principals
		}
	}
	if updatedPolicy.Statement[0].Principal.AWS == nil && updatedPolicy.Statement[1].Principal.AWS == nil {
		_, err = s.S3.DeleteBucketPolicy(&s3.DeleteBucketPolicyInput{
			Bucket: aws.String(fullBucketName),
		})
		return updatedPolicy, err
	}
	updatedPolicyJSON, err := json.Marshal(updatedPolicy)
	if err != nil {
		return updatedPolicy, err
	}
	err = s.putBucketPolicyWithTimeout(fullBucketName, string(updatedPolicyJSON))
	if err != nil {
		return updatedPolicy, err
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
			Action: []string{
				"s3:GetObject",
				"s3:PutObject",
				"s3:DeleteObject",
			},
		},
		{
			Effect: "Allow",
			Action: []string{
				"s3:GetBucketLocation",
				"s3:ListBucket",
			},
		},
	},
}
