package s3

import (
	"encoding/json"
	"fmt"
	"time"

	"strings"

	"code.cloudfoundry.org/lager"
	"github.com/alphagov/paas-go/provider"
	"github.com/alphagov/paas-s3-broker/s3/policy"
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

type BindParams struct {
	Permissions         string `json:"permissions"`
	AllowExternalAccess bool   `json:"allow_external_access"`
}

type ProvisionParams struct {
	PublicBucket bool `json:"public_bucket"`
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
	bucketName := s.buildBucketName(provisionData.InstanceID)

	_, err := s.s3Client.CreateBucket(&s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	})

	if err != nil {
		return err
	}

	_, err = s.s3Client.PutBucketEncryption(&s3.PutBucketEncryptionInput{
		Bucket: aws.String(bucketName),
		ServerSideEncryptionConfiguration: &s3.ServerSideEncryptionConfiguration{
			Rules: []*s3.ServerSideEncryptionRule{
				{
					ApplyServerSideEncryptionByDefault: &s3.ServerSideEncryptionByDefault{
						SSEAlgorithm: aws.String(s3.ServerSideEncryptionAes256),
					},
				},
			},
		},
	})
	if err != nil {
		return err
	}

	provisionParams := ProvisionParams{
		PublicBucket: false,
	}
	if provisionData.Details.RawParameters != nil {
		err := json.Unmarshal(provisionData.Details.RawParameters, &provisionParams)
		if err != nil {
			return err
		}
	}
	if provisionParams.PublicBucket {
		var permissions policy.Permissions = policy.PublicBucketPermissions{}
		stmt := policy.BuildStatement(bucketName, iam.User{Arn: aws.String("*")}, permissions)
		initialBucketPolicy, err := policy.BuildPolicy("", stmt)
		if err != nil {
			return err
		}
		initialPolicyJSON, err := json.Marshal(initialBucketPolicy)
		if err != nil {
			return err
		}

		err = s.putBucketPolicyWithTimeout(s.buildBucketName(provisionData.InstanceID), string(initialPolicyJSON))
		if err != nil {
			return err
		}
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
		{
			Key:   aws.String("tenant"),
			Value: aws.String(provisionData.Details.OrganizationGUID),
		},
		{
			Key:   aws.String("chargeable_entity"),
			Value: aws.String(provisionData.InstanceID),
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
	var permissions policy.Permissions = policy.ReadWritePermissions{}

	bindParams := BindParams{
		AllowExternalAccess: false,
		Permissions: policy.ReadWritePermissionsName, // Required, as if another bind parameter is set, `ValidatePermissions` is called below.
	}
	if bindData.Details.RawParameters != nil {
		err := json.Unmarshal(bindData.Details.RawParameters, &bindParams)
		if err != nil {
			return BucketCredentials{}, err
		}

		permissions, err = policy.ValidatePermissions(bindParams.Permissions)
		if err != nil {
			return BucketCredentials{}, err
		}
	}

	fullBucketName := s.buildBucketName(bindData.InstanceID)
	username := s.buildBindingUsername(bindData.BindingID)
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

	if !bindParams.AllowExternalAccess {
		_, err = s.iamClient.AttachUserPolicy(&iam.AttachUserPolicyInput{
			PolicyArn: aws.String(s.ipRestrictionPolicyArn),
			UserName:  aws.String(username),
		})
		if err != nil {
			s.deleteUserWithoutError(username)
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

	stmt := policy.BuildStatement(fullBucketName, *createUserOutput.User, permissions)

	updatedBucketPolicy, err := policy.BuildPolicy(currentBucketPolicy, stmt)

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
	policies, err := s.iamClient.ListAttachedUserPolicies(&iam.ListAttachedUserPoliciesInput{
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
	if policies != nil {
		for _, p := range policies.AttachedPolicies {
			_, err := s.iamClient.DetachUserPolicy(&iam.DetachUserPolicyInput{
				UserName:  aws.String(username),
				PolicyArn: p.PolicyArn,
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

func (s *S3Client) buildBucketArns(bucketName string) (wildcardArn, bareArn string) {
	return fmt.Sprintf("arn:aws:s3:::%s/*", bucketName), fmt.Sprintf("arn:aws:s3:::%s", bucketName)
}

func (s *S3Client) buildBucketName(instanceID string) string {
	return fmt.Sprintf("%s%s", s.bucketPrefix, instanceID)
}

func (s *S3Client) buildBindingUsername(bindingID string) string {
	return fmt.Sprintf("%s%s", s.bucketPrefix, bindingID)
}

func (s *S3Client) RemoveUserFromBucketAndDeleteUser(bindingID, bucketName string) error {
	username := s.buildBindingUsername(bindingID)
	fullBucketName := s.buildBucketName(bucketName)

	getBucketPolicyOutput, err := s.s3Client.GetBucketPolicy(&s3.GetBucketPolicyInput{
		Bucket: aws.String(fullBucketName),
	})
	if err != nil {
		return err
	}

	updatedPolicy, err := policy.RemoveUserFromPolicy(*getBucketPolicyOutput.Policy, username)
	if err != nil {
		return err
	}

	err = s.deleteUser(username)
	if err != nil {
		return err
	}

	if len(updatedPolicy.Statement) > 0 {
		updatedPolicyJSON, err := json.Marshal(updatedPolicy)
		if err != nil {
			return err
		}

		return s.putBucketPolicyWithTimeout(fullBucketName, string(updatedPolicyJSON))
	} else {
		_, err = s.s3Client.DeleteBucketPolicy(&s3.DeleteBucketPolicyInput{
			Bucket: aws.String(fullBucketName),
		})
		return err
	}
}
