package s3

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"strings"

	"code.cloudfoundry.org/lager"
	"github.com/alphagov/paas-s3-broker/s3/policy"
	"github.com/alphagov/paas-service-broker-base/provider"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/iam/iamiface"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
)

const (
	awsMaxWaitAttempts = 15
	awsWaitDelay       = 3 * time.Second
)

var (
	ErrNoSuchResources = errors.New("no such resources found")
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -o fakes/fake_s3_client.go . Client
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

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -o fakes/fake_s3_api.go github.com/aws/aws-sdk-go/service/s3/s3iface.S3API
//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -o fakes/fake_iam_api.go github.com/aws/aws-sdk-go/service/iam/iamiface.IAMAPI

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
	context                context.Context
}

type BindParams struct {
	Permissions         string `json:"permissions"`
	AllowExternalAccess bool   `json:"allow_external_access"`
}

type ProvisionParams struct {
	PublicBucket bool `json:"public_bucket"`
}

func NewS3Client(
	config *Config,
	s3Client s3iface.S3API,
	iamClient iamiface.IAMAPI,
	logger lager.Logger,
	ctx context.Context,
) *S3Client {
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
		context:                ctx,
	}
}

func (s *S3Client) CreateBucket(provisionData provider.ProvisionData) error {
	logger := s.logger.Session("create-bucket")
	bucketName := s.buildBucketName(provisionData.InstanceID)

	logger.Info("create-bucket", lager.Data{"bucket": bucketName})
	_, err := s.s3Client.CreateBucket(&s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	})

	if err != nil {
		logger.Error("create-bucket", err)
		return err
	}

	err = s.s3Client.WaitUntilBucketExistsWithContext(
		s.context,
		&s3.HeadBucketInput{Bucket: aws.String(bucketName)},

		request.WithWaiterDelay(request.ConstantWaiterDelay(awsWaitDelay)),
		request.WithWaiterMaxAttempts(awsMaxWaitAttempts),
	)

	if err != nil {
		logger.Error("wait-until-bucket-exists", err)
		return err
	}

	logger.Info("put-public-access-block", lager.Data{"bucket": bucketName})
	_, err = s.s3Client.PutPublicAccessBlock(&s3.PutPublicAccessBlockInput{
		Bucket: aws.String(bucketName),
		PublicAccessBlockConfiguration: &s3.PublicAccessBlockConfiguration{
			BlockPublicAcls:       aws.Bool(true),
			BlockPublicPolicy:     aws.Bool(true),
			IgnorePublicAcls:      aws.Bool(true),
			RestrictPublicBuckets: aws.Bool(true),
		},
	})
	if err != nil {
		logger.Error("put-public-access-block", err)
		return err
	}

	logger.Info("put-bucket-encryption", lager.Data{"bucket": bucketName, "sse-algorithm": s3.ServerSideEncryptionAes256})
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
		logger.Error("put-bucket-encryption", err)
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
		logger.Info("delete-public-access-block", lager.Data{"bucket": bucketName})
		_, err = s.s3Client.DeletePublicAccessBlock(&s3.DeletePublicAccessBlockInput{
			Bucket: aws.String(bucketName),
		})
		if err != nil {
			logger.Error("delete-public-access-block", err)
			return err
		}

		logger.Info("make-bucket-public", lager.Data{"bucket": bucketName})
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
			logger.Error("make-bucket-public", err)
			return err
		}
	}

	tags := []*s3.Tag{
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
	}
	logger.Info("tag-bucket", lager.Data{"bucket": bucketName, "tags": tags})
	_, err = s.tagBucket(provisionData.InstanceID, tags)
	if err != nil {
		logger.Error("tag-bucket", err)
		logger.Info("delete-bucket", lager.Data{"bucket": bucketName})
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
	logger := s.logger.Session("delete-bucket")
	fullBucketName := s.buildBucketName(name)

	logger.Info("delete-bucket", lager.Data{"bucket": fullBucketName})
	_, err := s.s3Client.DeleteBucket(&s3.DeleteBucketInput{
		Bucket: aws.String(fullBucketName),
	})
	if err != nil {
		logger.Error("delete-bucket", err)
		if awsErr, ok := err.(awserr.Error); ok && awsErr.Code() == "NoSuchBucket" {
			return ErrNoSuchResources
		}
	}
	return err
}

func (s *S3Client) AddUserToBucket(bindData provider.BindData) (BucketCredentials, error) {
	logger := s.logger.Session("add-user-to-bucket")
	var permissions policy.Permissions = policy.ReadWritePermissions{}

	bindParams := BindParams{
		AllowExternalAccess: false,
		Permissions:         policy.ReadWritePermissionsName, // Required, as if another bind parameter is set, `ValidatePermissions` is called below.
	}
	if bindData.Details.RawParameters != nil {
		logger.Info("parse-raw-params")
		err := json.Unmarshal(bindData.Details.RawParameters, &bindParams)
		if err != nil {
			logger.Error("parse-raw-params", err)
			return BucketCredentials{}, err
		}

		permissions, err = policy.ValidatePermissions(bindParams.Permissions)
		if err != nil {
			logger.Error("invalid-permissions", err)
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

	user := &iam.CreateUserInput{
		Path:     aws.String(s.iamUserPath),
		UserName: aws.String(username),
		Tags:     userTags,
	}
	logger.Info("create-user", lager.Data{"bucket": fullBucketName, "user": user})
	createUserOutput, err := s.iamClient.CreateUser(user)
	if err != nil {
		logger.Error("create-user", err)
		return BucketCredentials{}, err
	}

	err = s.iamClient.WaitUntilUserExistsWithContext(
		s.context,
		&iam.GetUserInput{UserName: aws.String(username)},

		request.WithWaiterDelay(request.ConstantWaiterDelay(awsWaitDelay)),
		request.WithWaiterMaxAttempts(awsMaxWaitAttempts),
	)

	if err != nil {
		logger.Error("wait-for-user-exist", err)
		return BucketCredentials{}, err
	}

	if !bindParams.AllowExternalAccess {
		logger.Info("allow-external-access", lager.Data{"bucket": fullBucketName})
		_, err = s.iamClient.AttachUserPolicy(&iam.AttachUserPolicyInput{
			PolicyArn: aws.String(s.ipRestrictionPolicyArn),
			UserName:  aws.String(username),
		})
		if err != nil {
			logger.Error("allow-external-access", err)
			s.deleteUserWithoutError(username)
			return BucketCredentials{}, err
		}
	}

	logger.Info("create-access-key", lager.Data{"bucket": fullBucketName, "username": username})
	createAccessKeyOutput, err := s.iamClient.CreateAccessKey(&iam.CreateAccessKeyInput{
		UserName: aws.String(username),
	})
	if err != nil {
		logger.Error("create-access-key", err)
		s.deleteUserWithoutError(username)
		return BucketCredentials{}, err
	}

	logger.Info("get-bucket-policy", lager.Data{"bucket": fullBucketName})
	getBucketPolicyOutput, err := s.s3Client.GetBucketPolicy(&s3.GetBucketPolicyInput{
		Bucket: aws.String(fullBucketName),
	})
	currentBucketPolicy := ""
	if err != nil {
		if !strings.Contains(err.Error(), "NoSuchBucketPolicy: The bucket policy does not exist") {
			logger.Error("get-bucket-policy", err)
			s.deleteUserWithoutError(username)
			return BucketCredentials{}, err
		}
	} else {
		currentBucketPolicy = *getBucketPolicyOutput.Policy
	}

	stmt := policy.BuildStatement(fullBucketName, *createUserOutput.User, permissions)

	logger.Info("update-bucket-policy", lager.Data{"bucket": fullBucketName})
	updatedBucketPolicy, err := policy.BuildPolicy(currentBucketPolicy, stmt)
	if err != nil {
		s.deleteUserWithoutError(username)
		return BucketCredentials{}, err
	}

	updatedPolicyJSON, err := json.Marshal(updatedBucketPolicy)
	if err != nil {
		logger.Error("update-bucket-policy", err)
		s.deleteUserWithoutError(username)
		return BucketCredentials{}, err
	}

	err = s.putBucketPolicyWithTimeout(fullBucketName, string(updatedPolicyJSON))
	if err != nil {
		logger.Error("update-bucket-policy", err)
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

			time.Sleep(2 * time.Second)

			if apiErr == nil {
				return apiErr
			}
		}
	}
}

func (s *S3Client) deleteUserWithoutError(username string) {
	err := s.deleteUser(username)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Deleted User %s, and suppressed error", username), err)
	}
}

func isIAMUserNotFound(err error) bool {
	// if we only have path-restricted permissions on IAM users (as is
	// the recommended configuration), a non-existent user will be
	// indistinguishable from one we don't have permission to see because
	// a non-existent user has no path to qualify us to be able to see it
	awsErr, ok := err.(awserr.Error)
	return (!ok) || (awsErr.Code() != iam.ErrCodeNoSuchEntityException && awsErr.Code() != "AccessDenied")
}

func (s *S3Client) deleteUser(username string) error {
	hadEffect := false

	var (
		keys     []*iam.AccessKeyMetadata
		policies []*iam.AttachedPolicy
	)

	keysOutput, err := s.iamClient.ListAccessKeys(&iam.ListAccessKeysInput{
		UserName: aws.String(username),
	})
	if err != nil {
		s.logger.Error("list-access-keys", err)
		if isIAMUserNotFound(err) {
			return err
		}
	} else {
		keys = keysOutput.AccessKeyMetadata
	}

	policiesOutput, err := s.iamClient.ListAttachedUserPolicies(&iam.ListAttachedUserPoliciesInput{
		UserName: aws.String(username),
	})
	if err != nil {
		s.logger.Error("list-attached-user-policies", err)
		if isIAMUserNotFound(err) {
			return err
		}
	} else {
		policies = policiesOutput.AttachedPolicies
	}

	for _, k := range keys {
		_, err := s.iamClient.DeleteAccessKey(&iam.DeleteAccessKeyInput{
			UserName:    aws.String(username),
			AccessKeyId: k.AccessKeyId,
		})
		if err != nil {
			s.logger.Error("delete-access-key", err)
			return err
		}

		hadEffect = true
	}
	for _, p := range policies {
		_, err := s.iamClient.DetachUserPolicy(&iam.DetachUserPolicyInput{
			UserName:  aws.String(username),
			PolicyArn: p.PolicyArn,
		})
		if err != nil {
			s.logger.Error("detach-user-policy", err)
			return err
		}

		hadEffect = true
	}

	_, err = s.iamClient.DeleteUser(&iam.DeleteUserInput{
		UserName: aws.String(username),
	})
	if err != nil {
		s.logger.Error("delete-user", err)
		if isIAMUserNotFound(err) {
			return err
		}
	} else {
		hadEffect = true
	}

	if !hadEffect {
		return ErrNoSuchResources
	}

	return nil
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
	logger := s.logger.Session("remove-user-from-bucket")

	hadEffect := false

	username := s.buildBindingUsername(bindingID)
	fullBucketName := s.buildBucketName(bucketName)

	logger.Info("get-bucket-policy", lager.Data{"bucket": fullBucketName})
	getBucketPolicyOutput, err := s.s3Client.GetBucketPolicy(&s3.GetBucketPolicyInput{
		Bucket: aws.String(fullBucketName),
	})
	if err != nil {
		logger.Error("get-bucket-policy", err)
		if awsErr, ok := err.(awserr.Error); (!ok) || awsErr.Code() != "NoSuchBucketPolicy" {
			return err
		}
	} else {
		logger.Info(
			"remove-user-from-policy",
			lager.Data{
				"bucket":   fullBucketName,
				"username": username,
			},
		)
		updatedPolicy, err := policy.RemoveUserFromPolicy(
			*getBucketPolicyOutput.Policy,
			username,
		)
		if err != nil {
			logger.Error("remove-user-from-policy", err)

			if !strings.Contains(err.Error(), "could not find a policy statement for user") {
				return err
			}
		} else {
			logger.Info(
				"policy-statements",
				lager.Data{
					"bucket": fullBucketName,
					"count":  len(updatedPolicy.Statement),
				},
			)
			if len(updatedPolicy.Statement) > 0 {
				logger.Info("update-policy", lager.Data{"bucket": fullBucketName})
				updatedPolicyJSON, err := json.Marshal(updatedPolicy)
				if err != nil {
					logger.Error("update-policy", err)
					return err
				}

				err = s.putBucketPolicyWithTimeout(
					fullBucketName,
					string(updatedPolicyJSON),
				)
				if err != nil {
					logger.Error("put-bucket-policy-with-timeout", err)
					return err
				}
			} else {
				logger.Info("delete-policy", lager.Data{"bucket": fullBucketName})
				_, err = s.s3Client.DeleteBucketPolicy(&s3.DeleteBucketPolicyInput{
					Bucket: aws.String(fullBucketName),
				})
				if err != nil {
					logger.Error("delete-policy", err)
					return err
				}
			}
			hadEffect = true
		}
	}

	logger.Info("delete-user", lager.Data{"username": username})
	err = s.deleteUser(username)
	if err != nil {
		logger.Error("delete-user", err)
		if err != ErrNoSuchResources {
			return err
		}
	} else {
		hadEffect = true
	}

	if !hadEffect {
		return ErrNoSuchResources
	}

	return nil
}
