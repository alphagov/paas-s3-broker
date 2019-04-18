package s3

import (
	"encoding/json"
	"fmt"
	"time"

	"strings"

	"code.cloudfoundry.org/lager"
	"github.com/alphagov/paas-go/provider"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/iam/iamiface"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
)

//go:generate counterfeiter -o fakes/fake_s3_client.go . Client
type Client interface {
	CreateBucket(provisionData provider.ProvisionData) error
	DeleteBucket(name string) error
	AddUserToBucket(bindData provider.BindData) (BucketCredentials, error)
	RemoveUserFromBucketAndDeleteUser(bindingID, bucketName string) error
}

type BucketCredentials struct {
	BucketName         string `json:"bucket_name"`
	AWSAccessKeyID     string `json:"aws_access_key_id"`
	AWSSecretAccessKey string `json:"aws_secret_access_key"`
	AWSRegion          string `json:"aws_region"`
	DeployEnvironment  string `json:"deploy_env"`
}

type Config struct {
	AWSRegion              string `json:"aws_region"`
	ResourcePrefix         string `json:"resource_prefix"`
	IAMUserPath            string `json:"iam_user_path"`
	DeployEnvironment      string `json:"deploy_env"`
	IpRestrictionPolicyARN string `json:"iam_ip_restriction_policy_arn"`
	Timeout                time.Duration
}

func NewS3ClientConfig(configJSON []byte) (*Config, error) {
	config := &Config{}
	err := json.Unmarshal(configJSON, &config)
	if err != nil {
		return nil, err
	}

	return config, nil
}

type S3Client struct {
	bucketPrefix           string
	iamUserPath            string
	ipRestrictionPolicyArn string
	awsRegion              string
	deployEnvironment      string
	timeout                time.Duration
	s3Client               s3iface.S3API
	iamClient              iamiface.IAMAPI
	logger                 lager.Logger
}

func NewS3Client(config *Config, s3Client s3iface.S3API, iamClient iamiface.IAMAPI, logger lager.Logger) *S3Client {
	timeout := config.Timeout
	if timeout == time.Duration(0) {
		timeout = 30 * time.Second
	}

	return &S3Client{
		bucketPrefix:           config.ResourcePrefix,
		iamUserPath:            fmt.Sprintf("/%s/", strings.Trim(config.IAMUserPath, "/")),
		ipRestrictionPolicyArn: config.IpRestrictionPolicyARN,
		awsRegion:              config.AWSRegion,
		deployEnvironment:      config.DeployEnvironment,
		timeout:                timeout,
		s3Client:               s3Client,
		iamClient:              iamClient,
		logger:                 logger,
	}
}

func (s *S3Client) CreateBucket(provisionData provider.ProvisionData) error {
	_, err := s.s3Client.CreateBucket(&s3.CreateBucketInput{
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
			Value: aws.String(s.deployEnvironment),
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
	_, err := s.s3Client.DeleteBucket(&s3.DeleteBucketInput{
		Bucket: aws.String(s.buildBucketName(name)),
	})
	return err
}

func (s *S3Client) AddUserToBucket(bindData provider.BindData) (BucketCredentials, error) {
	type extraBindParams struct {
		allowExternalAccess bool `json:"allow_external_access"`
	}
	var bindParams extraBindParams
	err := json.Unmarshal(bindData.Details.RawParameters, &bindParams)

	fullBucketName := s.buildBucketName(bindData.InstanceID)
	username := s.bucketPrefix + bindData.BindingID
	userTags := []*iam.Tag{
		{
			Key:   aws.String("service_instance_guid"),
			Value: aws.String(bindData.InstanceID),
		},
		{
			Key:   aws.String("created_by"),
			Value: aws.String("paas-s3-broker"),
		},
		{
			Key:   aws.String("deploy_env"),
			Value: aws.String(s.deployEnvironment),
		},
	}

	createUserOutput, err := s.iamClient.CreateUser(&iam.CreateUserInput{
		Path:     aws.String(s.iamUserPath),
		UserName: aws.String(username),
		Tags:     userTags,
	})
	if err != nil {
		return BucketCredentials{}, err
	}

	if !bindParams.allowExternalAccess {
		_, err = s.iamClient.AttachUserPolicy(&iam.AttachUserPolicyInput{
			PolicyArn: aws.String(s.ipRestrictionPolicyArn),
			UserName:  aws.String(username),
		})
		if err != nil {
			return BucketCredentials{}, err
		}
	}

	createAccessKeyOutput, err := s.iamClient.CreateAccessKey(&iam.CreateAccessKeyInput{
		UserName: aws.String(username),
	})
	if err != nil {
		s.deleteUserWithoutError(username)
		return BucketCredentials{}, err
	}

	getBucketPolicyOutput, err := s.s3Client.GetBucketPolicy(&s3.GetBucketPolicyInput{
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
		AWSRegion:          s.awsRegion,
	}, nil
}

func (s *S3Client) putBucketPolicyWithTimeout(fullBucketName, updatedPolicyJSON string) error {
	var apiErr error
	timeoutChannel := make(chan bool)
	go func() {
		time.Sleep(s.timeout)
		timeoutChannel <- true
	}()
	for {
		select {
		case <-timeoutChannel:
			return apiErr
		default:
			_, apiErr = s.s3Client.PutBucketPolicy(&s3.PutBucketPolicyInput{
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
	err := s.deleteUser(username)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Deleted User %s, and suppressed error", username), err)
	}
}

func (s *S3Client) deleteUser(username string) error {
	keys, err := s.iamClient.ListAccessKeys(&iam.ListAccessKeysInput{
		UserName: aws.String(username),
	})
	if err != nil {
		return err
	}
	if keys != nil {
		for _, k := range keys.AccessKeyMetadata {
			_, err := s.iamClient.DeleteAccessKey(&iam.DeleteAccessKeyInput{
				UserName:    aws.String(username),
				AccessKeyId: k.AccessKeyId,
			})
			if err != nil {
				return err
			}
		}
	}
	_, err = s.iamClient.DeleteUser(&iam.DeleteUserInput{
		UserName: aws.String(username),
	})
	return err
}

func (s *S3Client) tagBucket(instanceID string, tags []*s3.Tag) (output *s3.PutBucketTaggingOutput, err error) {
	createTagsInput := s3.PutBucketTaggingInput{
		Bucket:  aws.String(s.buildBucketName(instanceID)),
		Tagging: &s3.Tagging{TagSet: tags},
	}
	result, err := s.s3Client.PutBucketTagging(&createTagsInput)
	return result, err
}

func (s *S3Client) buildBucketArns(fullBucketName string) (wildcardArn, bareArn string) {
	return fmt.Sprintf("arn:aws:s3:::%s/*", fullBucketName), fmt.Sprintf("arn:aws:s3:::%s", fullBucketName)
}

func (s *S3Client) buildBucketName(bucketName string) string {
	return fmt.Sprintf("%s%s", s.bucketPrefix, bucketName)
}

func (s *S3Client) AddUserToBucketPolicy(userArn, fullBucketName, policyDocument string) (PolicyDocument, error) {
	updatedPolicy := BucketPolicyTemplate()
	if policyDocument == "" {
		updatedPolicy.Statement[0].Resource, updatedPolicy.Statement[1].Resource = s.buildBucketArns(fullBucketName)
		for policyStatementIndex := range updatedPolicy.Statement {
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
		var existingPrincipals []interface{}
		switch p := principal.(type) {
		case string:
			existingPrincipals = []interface{}{p}
		case []interface{}:
			existingPrincipals = p
		}

		var principals []string
		for _, existingPrincipal := range existingPrincipals {
			principals = append(principals, existingPrincipal.(string))
		}
		principals = append(
			principals,
			userArn,
		)

		updatedPolicy.Statement[policyStatementIndex].Principal.AWS = principals
	}
	return updatedPolicy, nil
}

func (s *S3Client) RemoveUserFromBucketAndDeleteUser(bindingID, bucketName string) error {
	username := s.bucketPrefix + bindingID
	fullBucketName := s.buildBucketName(bucketName)

	getBucketPolicyOutput, err := s.s3Client.GetBucketPolicy(&s3.GetBucketPolicyInput{
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
	var updatedPolicy PolicyDocument

	err := json.Unmarshal([]byte(currentBucketPolicy), &updatedPolicy)
	if err != nil {
		return updatedPolicy, err
	}
	for policyStatementIndex := range updatedPolicy.Statement {
		principal := updatedPolicy.Statement[policyStatementIndex].Principal.AWS
		var existingPrincipals []interface{}
		switch p := principal.(type) {
		case string:
			existingPrincipals = []interface{}{p}
		case []interface{}:
			existingPrincipals = p
		}

		var principals []string
		for _, existingPrincipal := range existingPrincipals {
			if !s.arnIsForUser(existingPrincipal.(string), username) {
				principals = append(principals, existingPrincipal.(string))
			}
		}
		if len(principals) > 0 {
			updatedPolicy.Statement[policyStatementIndex].Principal.AWS = principals
		} else {
			updatedPolicy.Statement[policyStatementIndex].Principal.AWS = nil
		}
	}
	if updatedPolicy.Statement[0].Principal.AWS == nil && updatedPolicy.Statement[1].Principal.AWS == nil {
		_, err = s.s3Client.DeleteBucketPolicy(&s3.DeleteBucketPolicyInput{
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

func BucketPolicyTemplate() PolicyDocument {
	return PolicyDocument{
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
}
