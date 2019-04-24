package s3_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/alphagov/paas-s3-broker/s3/policy"
	"github.com/pivotal-cf/brokerapi"
	"time"

	"code.cloudfoundry.org/lager"
	"github.com/alphagov/paas-go/provider"
	"github.com/alphagov/paas-s3-broker/s3"
	fakeClient "github.com/alphagov/paas-s3-broker/s3/fakes"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
	awsS3 "github.com/aws/aws-sdk-go/service/s3"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Client", func() {
	var (
		s3API          *fakeClient.FakeS3API
		iamAPI         *fakeClient.FakeIAMAPI
		s3Client       *s3.S3Client
		s3ClientConfig *s3.Config
		logger         lager.Logger
	)

	BeforeEach(func() {
		s3API = &fakeClient.FakeS3API{}
		iamAPI = &fakeClient.FakeIAMAPI{}
		logger = lager.NewLogger("s3-service-broker-test")
		s3ClientConfig = &s3.Config{
			AWSRegion:         "eu-west-2",
			ResourcePrefix:    "test-bucket-prefix-",
			IAMUserPath:       "/test-iam-path/",
			DeployEnvironment: "test-env",
			Timeout:           2 * time.Second,
		}
		s3Client = s3.NewS3Client(
			s3ClientConfig,
			s3API,
			iamAPI,
			logger,
		)
	})

	Describe("AddUserToBucket", func() {
		It("manages the user and bucket policy", func() {
			// Set up fake API
			iamAPI.CreateUserReturnsOnCall(0, &iam.CreateUserOutput{
				User: &iam.User{
					Arn: aws.String("arn"),
				},
			}, nil)
			iamAPI.CreateAccessKeyReturnsOnCall(0, &iam.CreateAccessKeyOutput{
				AccessKey: &iam.AccessKey{
					AccessKeyId:     aws.String("access-key-id"),
					SecretAccessKey: aws.String("secret-access-key"),
				},
			}, nil)
			s3API.GetBucketPolicyReturnsOnCall(0, &awsS3.GetBucketPolicyOutput{
				Policy: aws.String(`{"Version": "2012-10-17", "Statement":[]}`),
			}, nil)
			bindData := provider.BindData{
				InstanceID: "test-instance-id",
				BindingID:  "test-binding-id",
			}
			bucketCredentials, err := s3Client.AddUserToBucket(bindData)
			Expect(err).NotTo(HaveOccurred())

			By("creating a user")
			Expect(iamAPI.CreateUserCallCount()).To(Equal(1))

			By("creating access keys for the user")
			Expect(iamAPI.CreateAccessKeyCallCount()).To(Equal(1))

			By("getting the bucket policy")
			Expect(s3API.GetBucketPolicyCallCount()).To(Equal(1))

			By("putting the updated policy")
			Expect(s3API.PutBucketPolicyCallCount()).To(Equal(1))

			By("with the right permissions")
			updatedPolicyInput := s3API.PutBucketPolicyArgsForCall(0)
			updatedPolicyStr := updatedPolicyInput.Policy

			updatedPolicy := policy.PolicyDocument{}
			err = json.Unmarshal([]byte(*updatedPolicyStr), &updatedPolicy)
			Expect(err).ToNot(HaveOccurred())
			Expect(updatedPolicy.Statement).To(HaveLen(1))
			Expect(updatedPolicy.Statement[0].Action).To(ContainElement("s3:PutObject"))
			Expect(updatedPolicy.Statement[0].Action).To(ContainElement("s3:GetObject"))

			By("returning the bucket credentials")
			Expect(bucketCredentials).To(Equal(s3.BucketCredentials{
				BucketName:         s3ClientConfig.ResourcePrefix + bindData.InstanceID,
				AWSAccessKeyID:     "access-key-id",
				AWSSecretAccessKey: "secret-access-key",
				AWSRegion:          s3ClientConfig.AWSRegion,
			}))
		})

		It("returns an error if the permissions requested aren't known", func() {
			// Set up fake API
			iamAPI.CreateUserReturnsOnCall(0, &iam.CreateUserOutput{
				User: &iam.User{
					Arn: aws.String("arn"),
				},
			}, nil)
			iamAPI.CreateAccessKeyReturnsOnCall(0, &iam.CreateAccessKeyOutput{
				AccessKey: &iam.AccessKey{
					AccessKeyId:     aws.String("access-key-id"),
					SecretAccessKey: aws.String("secret-access-key"),
				},
			}, nil)
			s3API.GetBucketPolicyReturnsOnCall(0, &awsS3.GetBucketPolicyOutput{
				Policy: aws.String(`{"Version": "2012-10-17", "Statement":[]}`),
			}, nil)

			bindData := provider.BindData{
				InstanceID: "test-instance-id",
				BindingID:  "test-binding-id",
				Details: brokerapi.BindDetails{
					RawParameters: json.RawMessage(`{"permissions": "read-write-banana"}`),
				},
			}

			_, err := s3Client.AddUserToBucket(bindData)
			Expect(err).To(HaveOccurred())
		})

		It("creates a policy with the requested permissions", func() {
			// Set up fake API
			iamAPI.CreateUserReturnsOnCall(0, &iam.CreateUserOutput{
				User: &iam.User{
					Arn: aws.String("arn"),
				},
			}, nil)
			iamAPI.CreateAccessKeyReturnsOnCall(0, &iam.CreateAccessKeyOutput{
				AccessKey: &iam.AccessKey{
					AccessKeyId:     aws.String("access-key-id"),
					SecretAccessKey: aws.String("secret-access-key"),
				},
			}, nil)
			s3API.GetBucketPolicyReturnsOnCall(0, &awsS3.GetBucketPolicyOutput{
				Policy: aws.String(`{"Version": "2012-10-17", "Statement":[]}`),
			}, nil)
			bindData := provider.BindData{
				InstanceID: "test-instance-id",
				BindingID:  "test-binding-id",
				Details: brokerapi.BindDetails{
					RawParameters: json.RawMessage(`{"permissions": "read-only"}`),
				},
			}
			bucketCredentials, err := s3Client.AddUserToBucket(bindData)
			Expect(err).NotTo(HaveOccurred())

			By("creating a user")
			Expect(iamAPI.CreateUserCallCount()).To(Equal(1))

			By("creating access keys for the user")
			Expect(iamAPI.CreateAccessKeyCallCount()).To(Equal(1))

			By("getting the bucket policy")
			Expect(s3API.GetBucketPolicyCallCount()).To(Equal(1))

			By("putting the updated policy")
			Expect(s3API.PutBucketPolicyCallCount()).To(Equal(1))

			By("with the right permissions")
			updatedPolicyInput := s3API.PutBucketPolicyArgsForCall(0)
			updatedPolicyStr := updatedPolicyInput.Policy

			updatedPolicy := policy.PolicyDocument{}
			err = json.Unmarshal([]byte(*updatedPolicyStr), &updatedPolicy)
			Expect(err).ToNot(HaveOccurred())
			Expect(updatedPolicy.Statement).To(HaveLen(1))
			Expect(updatedPolicy.Statement[0].Action).ToNot(ContainElement("s3:PutObject"))
			Expect(updatedPolicy.Statement[0].Action).To(ContainElement("s3:GetObject"))

			By("returning the bucket credentials")
			Expect(bucketCredentials).To(Equal(s3.BucketCredentials{
				BucketName:         s3ClientConfig.ResourcePrefix + bindData.InstanceID,
				AWSAccessKeyID:     "access-key-id",
				AWSSecretAccessKey: "secret-access-key",
				AWSRegion:          s3ClientConfig.AWSRegion,
			}))
		})

		It("does not create a policy with the bad permissions", func() {
			// Set up fake API
			iamAPI.CreateUserReturnsOnCall(0, &iam.CreateUserOutput{
				User: &iam.User{
					Arn: aws.String("arn"),
				},
			}, nil)
			iamAPI.CreateAccessKeyReturnsOnCall(0, &iam.CreateAccessKeyOutput{
				AccessKey: &iam.AccessKey{
					AccessKeyId:     aws.String("access-key-id"),
					SecretAccessKey: aws.String("secret-access-key"),
				},
			}, nil)
			s3API.GetBucketPolicyReturnsOnCall(0, &awsS3.GetBucketPolicyOutput{
				Policy: aws.String(`{"Version": "2012-10-17", "Statement":[]}`),
			}, nil)
			bindData := provider.BindData{
				InstanceID: "test-instance-id",
				BindingID:  "test-binding-id",
				Details: brokerapi.BindDetails{
					RawParameters: json.RawMessage(`{"permissions": "invalid-perms"}`),
				},
			}
			_, err := s3Client.AddUserToBucket(bindData)
			Expect(err).To(HaveOccurred())
		})

		Context("when creating an access key fails", func() {
			It("deletes the user", func() {
				// Set up fake API
				iamAPI.CreateUserReturnsOnCall(0, &iam.CreateUserOutput{
					User: &iam.User{},
				}, nil)
				iamAPI.CreateAccessKeyReturnsOnCall(0, &iam.CreateAccessKeyOutput{}, errors.New("some-error"))
				bindData := provider.BindData{
					InstanceID: "test-instance-id",
					BindingID:  "test-binding-id",
				}
				_, err := s3Client.AddUserToBucket(bindData)
				Expect(err).To(HaveOccurred())
				Expect(iamAPI.DeleteUserCallCount()).To(Equal(1))
			})
		})

		Context("when getting the bucket policy fails because it does not exist", func() {
			It("proceeds - this is expected for newly created buckets", func() {
				// Set up fake API
				iamAPI.CreateUserReturnsOnCall(0, &iam.CreateUserOutput{
					User: &iam.User{
						Arn: aws.String("arn"),
					},
				}, nil)
				iamAPI.CreateAccessKeyReturnsOnCall(0, &iam.CreateAccessKeyOutput{
					AccessKey: &iam.AccessKey{
						AccessKeyId:     aws.String("access-key-id"),
						SecretAccessKey: aws.String("secret-access-key"),
					},
				}, nil)
				s3API.GetBucketPolicyReturnsOnCall(0, &awsS3.GetBucketPolicyOutput{}, errors.New("NoSuchBucketPolicy: The bucket policy does not exist"))

				bindData := provider.BindData{
					InstanceID: "test-instance-id",
					BindingID:  "test-binding-id",
				}
				bucketCredentials, err := s3Client.AddUserToBucket(bindData)
				Expect(err).NotTo(HaveOccurred())
				Expect(bucketCredentials).To(Equal(s3.BucketCredentials{
					BucketName:         s3ClientConfig.ResourcePrefix + bindData.InstanceID,
					AWSAccessKeyID:     "access-key-id",
					AWSSecretAccessKey: "secret-access-key",
					AWSRegion:          s3ClientConfig.AWSRegion,
				}))
			})
		})

		Context("when getting the bucket policy fails for some generic reason", func() {
			It("deletes the user", func() {
				// Set up fake API
				iamAPI.CreateUserReturnsOnCall(0, &iam.CreateUserOutput{
					User: &iam.User{
						Arn: aws.String("arn"),
					},
				}, nil)
				iamAPI.CreateAccessKeyReturnsOnCall(0, &iam.CreateAccessKeyOutput{
					AccessKey: &iam.AccessKey{
						AccessKeyId:     aws.String("access-key-id"),
						SecretAccessKey: aws.String("secret-access-key"),
					},
				}, nil)
				s3API.GetBucketPolicyReturnsOnCall(0, &awsS3.GetBucketPolicyOutput{}, errors.New("some-error"))
				iamAPI.ListAccessKeysReturnsOnCall(0, &iam.ListAccessKeysOutput{
					AccessKeyMetadata: []*iam.AccessKeyMetadata{{AccessKeyId: aws.String("key")}},
				}, nil)
				iamAPI.DeleteAccessKeyReturnsOnCall(0, nil, nil)

				bindData := provider.BindData{
					InstanceID: "test-instance-id",
					BindingID:  "test-binding-id",
				}
				_, err := s3Client.AddUserToBucket(bindData)
				Expect(err).To(HaveOccurred())
				Expect(iamAPI.DeleteUserCallCount()).To(Equal(1))
				Expect(iamAPI.DeleteAccessKeyCallCount()).To(Equal(1))
			})
		})

		Context("when updating the bucket policy fails", func() {
			It("deletes the user", func() {
				// Set up fake API
				iamAPI.CreateUserReturnsOnCall(0, &iam.CreateUserOutput{
					User: &iam.User{
						Arn: aws.String("arn"),
					},
				}, nil)
				iamAPI.CreateAccessKeyReturnsOnCall(0, &iam.CreateAccessKeyOutput{
					AccessKey: &iam.AccessKey{},
				}, nil)
				s3API.GetBucketPolicyReturnsOnCall(0, &awsS3.GetBucketPolicyOutput{
					Policy: aws.String(`{""}`),
				}, nil)
				s3API.PutBucketPolicyReturns(&awsS3.PutBucketPolicyOutput{}, errors.New("some-error"))
				iamAPI.ListAccessKeysReturnsOnCall(0, &iam.ListAccessKeysOutput{
					AccessKeyMetadata: []*iam.AccessKeyMetadata{{AccessKeyId: aws.String("key")}},
				}, nil)
				iamAPI.DeleteAccessKeyReturnsOnCall(0, nil, nil)

				bindData := provider.BindData{
					InstanceID: "test-instance-id",
					BindingID:  "test-binding-id",
				}
				_, err := s3Client.AddUserToBucket(bindData)
				Expect(err).To(HaveOccurred())
				Expect(iamAPI.DeleteUserCallCount()).To(Equal(1))
				Expect(iamAPI.DeleteAccessKeyCallCount()).To(Equal(1))
			})
		})
	})

	Describe("RemoveUserFromBucketAndDeleteUser", func() {
		It("manages the user and bucket policy", func() {
			// Set up fake API
			userArn := "arn:aws:iam::account-number:user/s3-broker/" + s3ClientConfig.ResourcePrefix + "some-user"
			s3API.GetBucketPolicyReturnsOnCall(0, &awsS3.GetBucketPolicyOutput{
				Policy: aws.String(fmt.Sprintf(`
					{
						"Statement": [
							{
								"Action": [
									"s3:GetObject",
									"s3:PutObject",
									"s3:DeleteObject"
								],
								"Effect": "Allow",
								"Resource": [
									"arn:aws:s3:::gds-paas-s3-broker-bucketName",
									"arn:aws:s3:::gds-paas-s3-broker-bucketName/*"
								],
								"Principal": {
									"AWS": "%s"
								}
							}
						]
					}`, userArn)),
			}, nil)
			s3API.DeleteBucketPolicyReturnsOnCall(0, &awsS3.DeleteBucketPolicyOutput{}, nil)
			iamAPI.ListAccessKeysReturnsOnCall(0, &iam.ListAccessKeysOutput{
				AccessKeyMetadata: []*iam.AccessKeyMetadata{{AccessKeyId: aws.String("key")}},
			}, nil)
			iamAPI.DeleteAccessKeyReturnsOnCall(0, nil, nil)

			err := s3Client.RemoveUserFromBucketAndDeleteUser("some-user", "bucketName")
			Expect(err).NotTo(HaveOccurred())

			By("getting the bucket policy")
			Expect(s3API.GetBucketPolicyCallCount()).To(Equal(1))

			By("deleting the user and keys")
			Expect(iamAPI.DeleteUserCallCount()).To(Equal(1))
			Expect(iamAPI.DeleteAccessKeyCallCount()).To(Equal(1))

			By("deleting the bucket policy")
			Expect(s3API.DeleteBucketPolicyCallCount()).To(Equal(1))
		})

		It("updates the bucket policy when there are >0 statements left after unbinding", func() {
			// Set up fake API
			userArn := "arn:aws:iam::account-number:user/s3-broker/" + s3ClientConfig.ResourcePrefix + "some-user"
			s3API.GetBucketPolicyReturnsOnCall(0, &awsS3.GetBucketPolicyOutput{
				Policy: aws.String(fmt.Sprintf(`
					{
						"Statement": [
							{
								"Action": [
									"s3:GetObject",
									"s3:PutObject",
									"s3:DeleteObject"
								],
								"Effect": "Allow",
								"Resource": [
									"arn:aws:s3:::gds-paas-s3-broker-bucketName",
									"arn:aws:s3:::gds-paas-s3-broker-bucketName/*"
								],
								"Principal": {
									"AWS": "%s"
								}
							},
							{
								"Action": [
									"s3:GetObject",
									"s3:PutObject",
									"s3:DeleteObject"
								],
								"Effect": "Allow",
								"Resource": [
									"arn:aws:s3:::gds-paas-s3-broker-bucketName",
									"arn:aws:s3:::gds-paas-s3-broker-bucketName/*"
								],
								"Principal": {
									"AWS": "some-arn"
								}
							}
						]
					}`, userArn)),
			}, nil)
			s3API.PutBucketPolicyReturnsOnCall(0, &awsS3.PutBucketPolicyOutput{}, nil)
			iamAPI.ListAccessKeysReturnsOnCall(0, &iam.ListAccessKeysOutput{
				AccessKeyMetadata: []*iam.AccessKeyMetadata{{AccessKeyId: aws.String("key")}},
			}, nil)
			iamAPI.DeleteAccessKeyReturnsOnCall(0, nil, nil)

			err := s3Client.RemoveUserFromBucketAndDeleteUser("some-user", "bucketName")
			Expect(err).NotTo(HaveOccurred())

			By("getting the bucket policy")
			Expect(s3API.GetBucketPolicyCallCount()).To(Equal(1))

			By("deleting the user and keys")
			Expect(iamAPI.DeleteUserCallCount()).To(Equal(1))
			Expect(iamAPI.DeleteAccessKeyCallCount()).To(Equal(1))

			By("updating the bucket policy")
			Expect(s3API.PutBucketPolicyCallCount()).To(Equal(1))
			Expect(s3API.DeleteBucketPolicyCallCount()).To(Equal(0))
		})

		Context("when getting the bucket policy fails", func() {
			It("returns an error", func() {
				// Set up fake API
				errGettingPolicy := errors.New("error-getting-policy")
				s3API.GetBucketPolicyReturnsOnCall(0, &awsS3.GetBucketPolicyOutput{}, errGettingPolicy)

				err := s3Client.RemoveUserFromBucketAndDeleteUser("some-user", "bucketName")
				Expect(err).To(MatchError(errGettingPolicy))
			})
		})

		Context("when deleting the user fails", func() {
			It("returns an error", func() {
				// Set up fake API
				userArn := "arn:aws:iam::account-number:user/s3-broker/" + s3ClientConfig.ResourcePrefix + "some-user"
				s3API.GetBucketPolicyReturnsOnCall(0, &awsS3.GetBucketPolicyOutput{
					Policy: aws.String(fmt.Sprintf(`
					{
						"Statement": [
							{
								"Action": [
									"s3:GetObject",
									"s3:PutObject",
									"s3:DeleteObject"
								],
								"Effect": "Allow",
								"Resource": [
									"arn:aws:s3:::gds-paas-s3-broker-bucketName",
									"arn:aws:s3:::gds-paas-s3-broker-bucketName/*"
								],
								"Principal": {
									"AWS": "%s"
								}
							}
						]
					}`, userArn)),
				}, nil)

				s3API.PutBucketPolicyReturns(&awsS3.PutBucketPolicyOutput{}, nil)

				errDeletingUser := errors.New("error-deleting-user")
				iamAPI.DeleteUserReturnsOnCall(0, &iam.DeleteUserOutput{}, errDeletingUser)

				err := s3Client.RemoveUserFromBucketAndDeleteUser("some-user", "bucketName")
				Expect(err).To(MatchError(errDeletingUser))
			})
		})
	})
})
