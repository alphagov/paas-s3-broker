package s3_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/alphagov/paas-s3-broker/s3/policy"
	"github.com/pivotal-cf/brokerapi"
	"time"

	"code.cloudfoundry.org/lager"
	locketmodels "code.cloudfoundry.org/locket/models"
	"github.com/alphagov/paas-s3-broker/s3"
	fakeClient "github.com/alphagov/paas-s3-broker/s3/fakes"
	"github.com/alphagov/paas-service-broker-base/provider"
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
		locket         *fakeClient.FakeLocketClient
	)

	BeforeEach(func() {
		s3API = &fakeClient.FakeS3API{}
		iamAPI = &fakeClient.FakeIAMAPI{}
		logger = lager.NewLogger("s3-service-broker-test")
		locket = &fakeClient.FakeLocketClient{}
		s3ClientConfig = &s3.Config{
			AWSRegion:              "eu-west-2",
			ResourcePrefix:         "test-bucket-prefix-",
			IAMUserPath:            "/test-iam-path/",
			DeployEnvironment:      "test-env",
			Timeout:                2 * time.Second,
			IpRestrictionPolicyARN: "test-ip-restriction-policy-arn",
		}
		s3Client = s3.NewS3Client(
			s3ClientConfig,
			s3API,
			iamAPI,
			logger,
			locket,
			context.Background(),
		)
	})
	Describe("CreateBucket", func() {
		It("enables encryption at rest", func() {
			pd := provider.ProvisionData{}
			s3Client.CreateBucket(pd)

			Expect(s3API.CreateBucketCallCount()).To(Equal(1))
			Expect(s3API.PutBucketEncryptionCallCount()).To(Equal(1))

			encryptionCallParams := s3API.PutBucketEncryptionArgsForCall(0)
			encryptionCfg := encryptionCallParams.ServerSideEncryptionConfiguration
			Expect(len(encryptionCfg.Rules)).To(Equal(1))

			encryptionRule := encryptionCfg.Rules[0]
			Expect(encryptionRule.ApplyServerSideEncryptionByDefault).ToNot(BeNil())
		})
		It("creates a public bucket when specified", func() {
			pd := provider.ProvisionData{
				InstanceID: "test-instance-id",
				Details: brokerapi.ProvisionDetails{
					RawParameters: json.RawMessage(`{"public_bucket": true}`),
				},
			}
			s3Client.CreateBucket(pd)

			Expect(s3API.CreateBucketCallCount()).To(Equal(1))
			Expect(s3API.PutBucketPolicyCallCount()).To(Equal(1))
			policyInput := s3API.PutBucketPolicyArgsForCall(0)
			policyDoc, err := getPolicyFromPolicyCall(policyInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(policyDoc.Statement).To(HaveLen(1))
			Expect(policyDoc.Statement[0].Action).To(ContainElement("s3:GetObject"))
			Expect(policyDoc.Statement[0].Principal.AWS).To(Equal("*"))
		})
		It("creates a private bucket when specified", func() {
			pd := provider.ProvisionData{
				InstanceID: "test-instance-id",
				Details: brokerapi.ProvisionDetails{
					RawParameters: json.RawMessage(`{"public_bucket": false}`),
				},
			}
			s3Client.CreateBucket(pd)

			Expect(s3API.CreateBucketCallCount()).To(Equal(1))
			Expect(s3API.PutBucketPolicyCallCount()).To(Equal(0))
		})
		It("creates a private bucket by default", func() {
			pd := provider.ProvisionData{
				InstanceID: "test-instance-id",
				Details: brokerapi.ProvisionDetails{
					RawParameters: nil,
				},
			}
			s3Client.CreateBucket(pd)

			Expect(s3API.CreateBucketCallCount()).To(Equal(1))
			Expect(s3API.PutBucketPolicyCallCount()).To(Equal(0))
		})
		It("tags the bucket appropriately", func() {
			pd := provider.ProvisionData{
				InstanceID: "test-instance-id",
				Details: brokerapi.ProvisionDetails{
					RawParameters:    nil,
					OrganizationGUID: "test-org-guid",
					SpaceGUID:        "test-space-guid",
				},
				Plan: brokerapi.ServicePlan{
					ID: "test-plan-guid",
				},
			}
			s3Client.CreateBucket(pd)

			Expect(s3API.CreateBucketCallCount()).To(Equal(1))
			taggingArgs := s3API.PutBucketTaggingArgsForCall(0)
			Expect(len(taggingArgs.Tagging.TagSet)).To(Equal(8))
			Expect(hasTag(taggingArgs.Tagging.TagSet, "service_instance_guid", pd.InstanceID)).To(BeTrue())
			Expect(hasTag(taggingArgs.Tagging.TagSet, "org_guid", pd.Details.OrganizationGUID)).To(BeTrue())
			Expect(hasTag(taggingArgs.Tagging.TagSet, "space_guid", pd.Details.SpaceGUID)).To(BeTrue())
			Expect(hasTag(taggingArgs.Tagging.TagSet, "created_by", "paas-s3-broker")).To(BeTrue())
			Expect(hasTag(taggingArgs.Tagging.TagSet, "plan_guid", pd.Plan.ID)).To(BeTrue())
			Expect(hasTag(taggingArgs.Tagging.TagSet, "deploy_env", s3ClientConfig.DeployEnvironment)).To(BeTrue())
			Expect(hasTag(taggingArgs.Tagging.TagSet, "tenant", pd.Details.OrganizationGUID)).To(BeTrue())
			Expect(hasTag(taggingArgs.Tagging.TagSet, "chargeable_entity", pd.InstanceID)).To(BeTrue())
		})
		It("deletes the bucket if tagging fails", func() {
			pd := provider.ProvisionData{
				InstanceID: "test-instance-id",
				Details: brokerapi.ProvisionDetails{
					RawParameters:    nil,
					OrganizationGUID: "test-org-guid",
					SpaceGUID:        "test-space-guid",
				},
				Plan: brokerapi.ServicePlan{
					ID: "test-plan-guid",
				},
			}
			s3API.PutBucketTaggingReturns(nil, errors.New("lol"))
			s3Client.CreateBucket(pd)

			Expect(s3API.CreateBucketCallCount()).To(Equal(1))
			Expect(s3API.PutBucketTaggingCallCount()).To(Equal(1))
			Expect(s3API.DeleteBucketCallCount()).To(Equal(1))
		})
		It("gets a lock on the bucket name and releases it once it's created", func() {
			pd := provider.ProvisionData{
				InstanceID: "fake-instance-id",
			}
			s3Client.CreateBucket(pd)

			Expect(locket.LockCallCount()).To(Equal(1))
			_, lockCallOne, _ := locket.LockArgsForCall(0)
			Expect(lockCallOne.Resource.Key).To(ContainSubstring("fake-instance-id"))

			Expect(locket.ReleaseCallCount()).To(Equal(1))
			_, releaseReqOne, _ := locket.ReleaseArgsForCall(0)
			Expect(releaseReqOne.Resource.Key).To(Equal(lockCallOne.Resource.Key))
		})

		It("waits for a lock on the bucket name and releases it once it's created", func() {
			pd := provider.ProvisionData{
				InstanceID: "fake-instance-id",
			}

			locket.LockReturnsOnCall(0, nil, locketmodels.ErrLockCollision)
			locket.LockReturnsOnCall(1, nil, nil)

			s3Client.CreateBucket(pd)

			Expect(locket.LockCallCount()).To(Equal(2))

			_, lockCallOne, _ := locket.LockArgsForCall(0)
			Expect(lockCallOne.Resource.Key).To(ContainSubstring("fake-instance-id"))

			_, lockCallTwo, _ := locket.LockArgsForCall(1)
			Expect(lockCallTwo.Resource.Key).To(ContainSubstring("fake-instance-id"))

			Expect(locket.ReleaseCallCount()).To(Equal(1))
			_, releaseReqOne, _ := locket.ReleaseArgsForCall(0)
			Expect(releaseReqOne.Resource.Key).To(Equal(lockCallOne.Resource.Key))
		})

		It("fails after waiting for many locks", func() {
			pd := provider.ProvisionData{
				InstanceID: "fake-instance-id",
			}

			locket.LockReturns(nil, locketmodels.ErrLockCollision)

			err := s3Client.CreateBucket(pd)

			Expect(err).To(HaveOccurred())

			Expect(locket.LockCallCount()).To(Equal(15))

			_, lockCallOne, _ := locket.LockArgsForCall(0)
			Expect(lockCallOne.Resource.Key).To(ContainSubstring("fake-instance-id"))

			Expect(locket.ReleaseCallCount()).To(Equal(0))
		})
	})
	Describe("AddUserToBucket", func() {
		BeforeEach(func() {
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

		})
		It("manages the user and bucket policy", func() {
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

		It("gets a lock on the bucket and releases it at the end", func() {
			bindData := provider.BindData{
				InstanceID: "fake-instance-id",
				BindingID:  "test-binding-id",
			}
			_, err := s3Client.AddUserToBucket(bindData)
			Expect(err).NotTo(HaveOccurred())

			Expect(locket.LockCallCount()).To(Equal(1))
			_, lockCallOne, _ := locket.LockArgsForCall(0)
			Expect(lockCallOne.Resource.Key).To(ContainSubstring("fake-instance-id"))

			Expect(locket.ReleaseCallCount()).To(Equal(1))
			_, releaseReqOne, _ := locket.ReleaseArgsForCall(0)
			Expect(releaseReqOne.Resource.Key).To(Equal(lockCallOne.Resource.Key))
		})

		Context("when failing to AttachUserPolicy", func() {
			It("deletes the user", func() {
				expectedError := errors.New("attaching user policy failed. lul.")
				iamAPI.AttachUserPolicyReturnsOnCall(0, &iam.AttachUserPolicyOutput{}, expectedError)
				bindData := provider.BindData{
					InstanceID: "test-instance-id",
					BindingID:  "test-binding-id",
				}
				_, err := s3Client.AddUserToBucket(bindData)
				Expect(iamAPI.AttachUserPolicyCallCount()).To(Equal(1))
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(expectedError))
				Expect(iamAPI.DeleteUserCallCount()).To(Equal(1))
			})
		})

		Context("when not allowing external access", func() {
			Context("by omitting the parameter", func() {
				It("attaches the IP-Restriction policy", func() {
					bindData := provider.BindData{
						BindingID: "test-instance-id",
						Details: brokerapi.BindDetails{
							RawParameters: nil,
						},
					}
					s3Client.AddUserToBucket(bindData)
					createUserInput := iamAPI.CreateUserArgsForCall(0)
					Expect(iamAPI.AttachUserPolicyCallCount()).To(Equal(1))
					attachPolicyArgs := iamAPI.AttachUserPolicyArgsForCall(0)

					Expect(*attachPolicyArgs.PolicyArn).To(Equal(s3ClientConfig.IpRestrictionPolicyARN))
					Expect(*attachPolicyArgs.UserName).To(Equal(*createUserInput.UserName))
				})
			})
			Context("by setting the parameter to false", func() {
				It("attaches the IP-Restriction policy", func() {
					bindData := provider.BindData{
						BindingID: "test-instance-id",
						Details: brokerapi.BindDetails{
							RawParameters: json.RawMessage(`{"allow_external_access": false}`),
						},
					}
					s3Client.AddUserToBucket(bindData)
					createUserInput := iamAPI.CreateUserArgsForCall(0)
					Expect(iamAPI.AttachUserPolicyCallCount()).To(Equal(1))
					attachPolicyArgs := iamAPI.AttachUserPolicyArgsForCall(0)

					Expect(*attachPolicyArgs.PolicyArn).To(Equal(s3ClientConfig.IpRestrictionPolicyARN))
					Expect(*attachPolicyArgs.UserName).To(Equal(*createUserInput.UserName))
				})
			})
		})

		Context("when allowing external access by setting the parameter to true", func() {
			It("does not attach the IP-Restriction policy", func() {
				bindData := provider.BindData{
					BindingID: "test-instance-id",
					Details: brokerapi.BindDetails{
						RawParameters: json.RawMessage(`{"allow_external_access": true}`),
					},
				}
				s3Client.AddUserToBucket(bindData)
				Expect(iamAPI.AttachUserPolicyCallCount()).To(Equal(0))
			})
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

		It("gets a lock on the bucket and releases it at the end", func() {
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

			err := s3Client.RemoveUserFromBucketAndDeleteUser("some-user", "fake-instance-id")
			Expect(err).NotTo(HaveOccurred())

			Expect(locket.LockCallCount()).To(Equal(1))
			_, lockCallOne, _ := locket.LockArgsForCall(0)
			Expect(lockCallOne.Resource.Key).To(ContainSubstring("fake-instance-id"))

			Expect(locket.ReleaseCallCount()).To(Equal(1))
			_, releaseReqOne, _ := locket.ReleaseArgsForCall(0)
			Expect(releaseReqOne.Resource.Key).To(Equal(lockCallOne.Resource.Key))
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

func getPolicyFromPolicyCall(input *awsS3.PutBucketPolicyInput) (policy.PolicyDocument, error) {
	policyStr := input.Policy

	policyDoc := policy.PolicyDocument{}
	err := json.Unmarshal([]byte(*policyStr), &policyDoc)
	return policyDoc, err
}

func hasTag(tags []*awsS3.Tag, key string, value string) bool {
	for _, tag := range tags {
		if *tag.Key == key && *tag.Value == value {
			return true
		}
	}
	return false
}
