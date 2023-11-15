package s3_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/alphagov/paas-s3-broker/s3/policy"
	"github.com/pivotal-cf/brokerapi/domain"

	"code.cloudfoundry.org/lager"
	"github.com/alphagov/paas-s3-broker/s3"
	fakeClient "github.com/alphagov/paas-s3-broker/s3/fakes"
	"github.com/alphagov/paas-service-broker-base/provider"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/iam"
	awsS3 "github.com/aws/aws-sdk-go/service/s3"
	. "github.com/onsi/ginkgo/v2"
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
		s3ClientConfig = &s3.Config{
			AWSRegion:              "eu-west-2",
			ResourcePrefix:         "test-bucket-prefix-",
			IAMUserPath:            "/test-iam-path/",
			DeployEnvironment:      "test-env",
			Timeout:                2 * time.Second,
			IpRestrictionPolicyARN: "test-ip-restriction-policy-arn",
		}
	})

	JustBeforeEach(func() {
		logger = lager.NewLogger("s3-service-broker-test")
		s3Client = s3.NewS3Client(
			s3ClientConfig,
			s3API,
			iamAPI,
			logger,
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
		It("sets the s3 public access block by default", func() {
			pd := provider.ProvisionData{
				InstanceID: "test-instance-id",
				Details: domain.ProvisionDetails{
					RawParameters: nil,
				},
			}
			s3Client.CreateBucket(pd)
			Expect(s3API.PutPublicAccessBlockCallCount()).To(Equal(1))
		})
		It("disables the s3 public access block when private", func() {
			pd := provider.ProvisionData{
				InstanceID: "test-instance-id",
				Details: domain.ProvisionDetails{
					RawParameters: json.RawMessage(`{"public_bucket": false}`),
				},
			}
			s3Client.CreateBucket(pd)
			Expect(s3API.PutPublicAccessBlockCallCount()).To(Equal(1))
		})
		It("deletes the s3 public access block when public", func() {
			pd := provider.ProvisionData{
				InstanceID: "test-instance-id",
				Details: domain.ProvisionDetails{
					RawParameters: json.RawMessage(`{"public_bucket": true}`),
				},
			}
			s3Client.CreateBucket(pd)
			Expect(s3API.PutPublicAccessBlockCallCount()).To(Equal(1))
			Expect(s3API.DeletePublicAccessBlockCallCount()).To(Equal(1))
		})
		It("creates a public bucket when specified", func() {
			pd := provider.ProvisionData{
				InstanceID: "test-instance-id",
				Details: domain.ProvisionDetails{
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
				Details: domain.ProvisionDetails{
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
				Details: domain.ProvisionDetails{
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
				Details: domain.ProvisionDetails{
					RawParameters:    nil,
					OrganizationGUID: "test-org-guid",
					SpaceGUID:        "test-space-guid",
				},
				Plan: domain.ServicePlan{
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
				Details: domain.ProvisionDetails{
					RawParameters:    nil,
					OrganizationGUID: "test-org-guid",
					SpaceGUID:        "test-space-guid",
				},
				Plan: domain.ServicePlan{
					ID: "test-plan-guid",
				},
			}
			s3API.PutBucketTaggingReturns(nil, errors.New("lol"))
			s3Client.CreateBucket(pd)

			Expect(s3API.CreateBucketCallCount()).To(Equal(1))
			Expect(s3API.PutBucketTaggingCallCount()).To(Equal(1))
			Expect(s3API.DeleteBucketCallCount()).To(Equal(1))
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
			createUserInput := iamAPI.CreateUserArgsForCall(0)
			Expect(createUserInput.UserName).To(HaveValue(Equal("test-bucket-prefix-test-binding-id")))
			Expect(createUserInput.Path).To(HaveValue(Equal("/test-iam-path/")))
			Expect(createUserInput.PermissionsBoundary).To(BeNil())
			Expect(createUserInput.Tags).To(Equal([]*iam.Tag{
				{
					Key:   aws.String("service_instance_guid"),
					Value: aws.String("test-instance-id"),
				},
				{
					Key:   aws.String("created_by"),
					Value: aws.String("paas-s3-broker"),
				},
				{
					Key:   aws.String("deploy_env"),
					Value: aws.String("test-env"),
				},
			}))

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

		It("handles unknown permissions", func() {
			bindData := provider.BindData{
				InstanceID: "test-instance-id",
				BindingID:  "test-binding-id",
				Details: domain.BindDetails{
					RawParameters: json.RawMessage(`{"permissions": "read-write-banana"}`),
				},
			}

			_, err := s3Client.AddUserToBucket(bindData)

			By("Not creating a user", func() {
				Expect(iamAPI.CreateUserCallCount()).To(Equal(0))
			})

			By("returning an error", func() {
				Expect(err).To(HaveOccurred())
			})
		})

		It("creates a bucket policy with the requested permissions", func() {
			bindData := provider.BindData{
				InstanceID: "test-instance-id",
				BindingID:  "test-binding-id",
				Details: domain.BindDetails{
					RawParameters: json.RawMessage(`{"permissions": "read-only"}`),
				},
			}
			bucketCredentials, err := s3Client.AddUserToBucket(bindData)
			Expect(err).NotTo(HaveOccurred())

			By("creating a user")
			Expect(iamAPI.CreateUserCallCount()).To(Equal(1))
			createUserInput := iamAPI.CreateUserArgsForCall(0)
			Expect(createUserInput.UserName).To(HaveValue(Equal("test-bucket-prefix-test-binding-id")))
			Expect(createUserInput.Path).To(HaveValue(Equal("/test-iam-path/")))
			Expect(createUserInput.PermissionsBoundary).To(BeNil())

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

		It("does not create a bucket policy with the bad permissions", func() {
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
				Details: domain.BindDetails{
					RawParameters: json.RawMessage(`{"permissions": "invalid-perms"}`),
				},
			}
			_, err := s3Client.AddUserToBucket(bindData)
			Expect(err).To(HaveOccurred())
		})

		Context("when a common user policy ARN is configured", func () {
			BeforeEach(func () {
				s3ClientConfig.CommonUserPolicyARN = "test-common-user-policy-arn"
			})

			It("attaches the common user policy", func() {
				bindData := provider.BindData{
					InstanceID: "test-instance-id",
					BindingID:  "test-binding-id",
					Details: domain.BindDetails{
						RawParameters: nil,
					},
				}
				_, err := s3Client.AddUserToBucket(bindData)
				Expect(err).ToNot(HaveOccurred())

				Expect(iamAPI.CreateUserCallCount()).To(Equal(1))
				createUserInput := iamAPI.CreateUserArgsForCall(0)
				Expect(iamAPI.AttachUserPolicyCallCount()).To(BeNumerically(">", 0))
				attachPolicyArgs := iamAPI.AttachUserPolicyArgsForCall(0)

				Expect(createUserInput.UserName).To(HaveValue(Equal("test-bucket-prefix-test-binding-id")))
				Expect(createUserInput.Path).To(HaveValue(Equal("/test-iam-path/")))
				Expect(createUserInput.PermissionsBoundary).To(BeNil())

				Expect(attachPolicyArgs.PolicyArn).To(HaveValue(Equal("test-common-user-policy-arn")))
				Expect(attachPolicyArgs.UserName).To(HaveValue(Equal(*createUserInput.UserName)))
			})

			Context("when AttachUserPolicy fails", func() {
				var expectedError error

				BeforeEach(func () {
					expectedError = errors.New("attaching user policy failed. lul.")
					iamAPI.AttachUserPolicyReturnsOnCall(0, &iam.AttachUserPolicyOutput{}, expectedError)
					iamAPI.ListAccessKeysReturnsOnCall(0, &iam.ListAccessKeysOutput{}, nil)
					iamAPI.ListAttachedUserPoliciesReturnsOnCall(0, &iam.ListAttachedUserPoliciesOutput{}, nil)
				})

				It("deletes the user", func() {
					bindData := provider.BindData{
						InstanceID: "test-instance-id",
						BindingID:  "test-binding-id",
					}
					_, err := s3Client.AddUserToBucket(bindData)
					Expect(err).To(HaveOccurred())
					Expect(err).To(Equal(expectedError))

					Expect(iamAPI.AttachUserPolicyCallCount()).To(Equal(1))
					Expect(iamAPI.DeleteAccessKeyCallCount()).To(Equal(0))
					Expect(iamAPI.DetachUserPolicyCallCount()).To(Equal(0))
					Expect(iamAPI.DeleteUserCallCount()).To(Equal(1))
				})
			})
		})

		Context("when a permissions boundary is configured", func() {
			BeforeEach(func () {
				s3ClientConfig.PermissionsBoundaryARN = "test-permissions-boundary-arn"
			})

			It("creates the user with the provided permissions boundary", func() {
				bindData := provider.BindData{
					InstanceID: "test-instance-id",
					BindingID:  "test-binding-id",
				}
				_, err := s3Client.AddUserToBucket(bindData)
				Expect(err).ToNot(HaveOccurred())

				Expect(iamAPI.CreateUserCallCount()).To(Equal(1))
				Expect(iamAPI.CreateAccessKeyCallCount()).To(Equal(1))
				Expect(s3API.GetBucketPolicyCallCount()).To(Equal(1))
				Expect(s3API.PutBucketPolicyCallCount()).To(Equal(1))

				createUserInput := iamAPI.CreateUserArgsForCall(0)
				Expect(createUserInput.UserName).To(HaveValue(Equal("test-bucket-prefix-test-binding-id")))
				Expect(createUserInput.Path).To(HaveValue(Equal("/test-iam-path/")))
				Expect(createUserInput.PermissionsBoundary).To(HaveValue(Equal("test-permissions-boundary-arn")))
			})
		})


		Context("when not allowing external access", func() {
			Context("when failing to AttachUserPolicy", func() {
				var expectedError error

				BeforeEach(func () {
					expectedError = errors.New("attaching user policy failed. lul.")
					iamAPI.AttachUserPolicyReturnsOnCall(0, &iam.AttachUserPolicyOutput{}, expectedError)
					iamAPI.ListAccessKeysReturnsOnCall(0, &iam.ListAccessKeysOutput{}, nil)
					iamAPI.ListAttachedUserPoliciesReturnsOnCall(0, &iam.ListAttachedUserPoliciesOutput{}, nil)
				})

				It("deletes the user", func() {
					bindData := provider.BindData{
						InstanceID: "test-instance-id",
						BindingID:  "test-binding-id",
					}
					_, err := s3Client.AddUserToBucket(bindData)
					Expect(err).To(HaveOccurred())
					Expect(err).To(Equal(expectedError))

					Expect(iamAPI.AttachUserPolicyCallCount()).To(Equal(1))
					Expect(iamAPI.DeleteAccessKeyCallCount()).To(Equal(0))
					Expect(iamAPI.DetachUserPolicyCallCount()).To(Equal(0))
					Expect(iamAPI.DeleteUserCallCount()).To(Equal(1))
				})
			})

			Context("by omitting the parameter", func() {
				It("attaches the IP-Restriction policy", func() {
					bindData := provider.BindData{
						BindingID: "test-instance-id",
						Details: domain.BindDetails{
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
						Details: domain.BindDetails{
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

			Context("when a common user policy ARN was configured", func () {
				BeforeEach(func () {
					s3ClientConfig.CommonUserPolicyARN = "test-common-user-policy-arn"
				})

				Context("when failing to AttachUserPolicy for the IP-restriction policy", func() {
					var expectedError error

					BeforeEach(func () {
						expectedError = errors.New("attaching user policy failed. lul.")
						iamAPI.AttachUserPolicyReturnsOnCall(1, &iam.AttachUserPolicyOutput{}, expectedError)
						iamAPI.ListAccessKeysReturnsOnCall(0, &iam.ListAccessKeysOutput{}, nil)
						iamAPI.ListAttachedUserPoliciesReturnsOnCall(0, &iam.ListAttachedUserPoliciesOutput{
							AttachedPolicies: []*iam.AttachedPolicy{
								&iam.AttachedPolicy{
									PolicyArn: aws.String("arn:aws:blah:blah:some-common-iam-policy-arn"),
									PolicyName: aws.String("my-common-iam-policy"),
								},
							},
							IsTruncated: aws.Bool(false),
						}, nil)
					})

					It("deletes the user", func() {
						bindData := provider.BindData{
							InstanceID: "test-instance-id",
							BindingID:  "test-binding-id",
						}
						_, err := s3Client.AddUserToBucket(bindData)
						Expect(err).To(HaveOccurred())
						Expect(err).To(Equal(expectedError))

						Expect(iamAPI.AttachUserPolicyCallCount()).To(Equal(2))
						attachPolicyArgs := iamAPI.AttachUserPolicyArgsForCall(0)
						Expect(iamAPI.DeleteAccessKeyCallCount()).To(Equal(0))
						Expect(iamAPI.DetachUserPolicyCallCount()).To(Equal(1))

						detachPolicyArgs := iamAPI.DetachUserPolicyArgsForCall(0)
						Expect(*detachPolicyArgs.PolicyArn).To(HaveValue(Equal("arn:aws:blah:blah:some-common-iam-policy-arn")))
						Expect(*detachPolicyArgs.UserName).To(Equal(*attachPolicyArgs.UserName))

						Expect(iamAPI.DeleteUserCallCount()).To(Equal(1))
					})
				})
			})
		})

		Context("when allowing external access by setting the parameter to true", func() {
			It("does not attach the IP-Restriction policy", func() {
				bindData := provider.BindData{
					BindingID: "test-instance-id",
					Details: domain.BindDetails{
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
				iamAPI.ListAccessKeysReturnsOnCall(0, &iam.ListAccessKeysOutput{}, nil)
				iamAPI.ListAttachedUserPoliciesReturnsOnCall(0, &iam.ListAttachedUserPoliciesOutput{}, nil)
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
				iamAPI.ListAttachedUserPoliciesReturnsOnCall(0, &iam.ListAttachedUserPoliciesOutput{
					AttachedPolicies: []*iam.AttachedPolicy{
						{
							PolicyArn:  aws.String("foo"),
							PolicyName: aws.String("bar"),
						},
					},
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
				iamAPI.ListAttachedUserPoliciesReturnsOnCall(0, &iam.ListAttachedUserPoliciesOutput{}, nil)
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
		It("deletes user and bucket policy when it is the only statement in the policy", func() {
			// Set up fake API
			userArn := "arn:aws:iam::account-number:user/s3-broker/" + s3ClientConfig.ResourcePrefix + "some-user"
			s3API.GetBucketPolicyReturns(&awsS3.GetBucketPolicyOutput{
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
			s3API.DeleteBucketPolicyReturns(&awsS3.DeleteBucketPolicyOutput{}, nil)
			iamAPI.ListAccessKeysReturns(&iam.ListAccessKeysOutput{
				AccessKeyMetadata: []*iam.AccessKeyMetadata{{AccessKeyId: aws.String("key")}},
			}, nil)
			iamAPI.ListAttachedUserPoliciesReturns(&iam.ListAttachedUserPoliciesOutput{}, nil)
			iamAPI.DeleteAccessKeyReturns(nil, nil)

			err := s3Client.RemoveUserFromBucketAndDeleteUser("some-user", "bucketName")
			Expect(err).NotTo(HaveOccurred())

			By("getting the bucket policy", func() {
				Expect(s3API.GetBucketPolicyCallCount()).To(Equal(1))
				Expect(s3API.GetBucketPolicyArgsForCall(0)).ToNot(BeNil())
				Expect(s3API.GetBucketPolicyArgsForCall(0).Bucket).ToNot(BeNil())
				Expect(s3API.GetBucketPolicyArgsForCall(0).Bucket).To(Equal(aws.String(s3ClientConfig.ResourcePrefix + "bucketName")))
			})

			By("deleting the bucket policy", func() {
				Expect(s3API.DeleteBucketPolicyCallCount()).To(Equal(1))
				Expect(s3API.DeleteBucketPolicyArgsForCall(0)).ToNot(BeNil())
				Expect(s3API.DeleteBucketPolicyArgsForCall(0).Bucket).ToNot(BeNil())
				Expect(s3API.DeleteBucketPolicyArgsForCall(0).Bucket).To(Equal(aws.String(s3ClientConfig.ResourcePrefix + "bucketName")))
			})

			By("deleting user keys and policies", func() {
				Expect(iamAPI.ListAccessKeysCallCount()).To(Equal(1))
				Expect(iamAPI.ListAccessKeysArgsForCall(0)).ToNot(BeNil())
				Expect(iamAPI.ListAccessKeysArgsForCall(0).UserName).ToNot(BeNil())
				Expect(iamAPI.ListAccessKeysArgsForCall(0).UserName).To(Equal(aws.String(s3ClientConfig.ResourcePrefix + "some-user")))

				Expect(iamAPI.ListAttachedUserPoliciesCallCount()).To(Equal(1))
				Expect(iamAPI.ListAttachedUserPoliciesArgsForCall(0)).ToNot(BeNil())
				Expect(iamAPI.ListAttachedUserPoliciesArgsForCall(0).UserName).ToNot(BeNil())
				Expect(iamAPI.ListAttachedUserPoliciesArgsForCall(0).UserName).To(Equal(aws.String(s3ClientConfig.ResourcePrefix + "some-user")))

				Expect(iamAPI.DeleteUserCallCount()).To(Equal(1))
				Expect(iamAPI.DeleteUserArgsForCall(0)).ToNot(BeNil())
				Expect(iamAPI.DeleteUserArgsForCall(0).UserName).ToNot(BeNil())
				Expect(iamAPI.DeleteUserArgsForCall(0).UserName).To(Equal(aws.String(s3ClientConfig.ResourcePrefix + "some-user")))
			})

			By("deleting the user", func() {
				Expect(iamAPI.DeleteAccessKeyCallCount()).To(Equal(1))
				Expect(iamAPI.DeleteAccessKeyArgsForCall(0)).ToNot(BeNil())
				Expect(iamAPI.DeleteAccessKeyArgsForCall(0).AccessKeyId).To(Equal(aws.String("key")))
				Expect(iamAPI.DeleteAccessKeyArgsForCall(0).UserName).To(Equal(aws.String(s3ClientConfig.ResourcePrefix + "some-user")))
			})

			// all calls accounted for
			Expect(s3API.Invocations()).To(HaveLen(2))
			Expect(iamAPI.Invocations()).To(HaveLen(4))
		})

		It("deletes the user and removes the associated statement from the bucket policy when it is not the only statement", func() {
			// Set up fake API
			userArn := "arn:aws:iam::account-number:user/s3-broker/" + s3ClientConfig.ResourcePrefix + "some-user"
			s3API.GetBucketPolicyReturns(&awsS3.GetBucketPolicyOutput{
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
									"AWS": "some-other-arn"
								}
							}
						]
					}`, userArn)),
			}, nil)
			s3API.PutBucketPolicyReturns(&awsS3.PutBucketPolicyOutput{}, nil)
			iamAPI.ListAccessKeysReturns(&iam.ListAccessKeysOutput{
				AccessKeyMetadata: []*iam.AccessKeyMetadata{{AccessKeyId: aws.String("key")}},
			}, nil)
			iamAPI.ListAttachedUserPoliciesReturns(&iam.ListAttachedUserPoliciesOutput{
				AttachedPolicies: []*iam.AttachedPolicy{
					{
						PolicyArn:  aws.String("foo"),
						PolicyName: aws.String("bar"),
					},
				},
			}, nil)
			iamAPI.DeleteAccessKeyReturns(nil, nil)

			err := s3Client.RemoveUserFromBucketAndDeleteUser("some-user", "bucketName")
			Expect(err).NotTo(HaveOccurred())

			By("getting the bucket policy", func() {
				Expect(s3API.GetBucketPolicyCallCount()).To(Equal(1))
				Expect(s3API.GetBucketPolicyArgsForCall(0)).ToNot(BeNil())
				Expect(s3API.GetBucketPolicyArgsForCall(0).Bucket).ToNot(BeNil())
				Expect(s3API.GetBucketPolicyArgsForCall(0).Bucket).To(Equal(aws.String(s3ClientConfig.ResourcePrefix + "bucketName")))
			})

			By("updating the bucket policy", func() {
				Expect(s3API.PutBucketPolicyCallCount()).To(Equal(1))
				Expect(s3API.PutBucketPolicyArgsForCall(0)).ToNot(BeNil())
				Expect(s3API.PutBucketPolicyArgsForCall(0).Bucket).To(Equal(aws.String(s3ClientConfig.ResourcePrefix + "bucketName")))
				Expect(s3API.PutBucketPolicyArgsForCall(0).Policy).ToNot(BeNil())
				Expect(aws.StringValue(s3API.PutBucketPolicyArgsForCall(0).Policy)).To(MatchJSON(`
					{
						"Version": "",
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
									"AWS": "some-other-arn"
								}
							}
						]
					}`))
				Expect(s3API.DeleteBucketPolicyCallCount()).To(Equal(0))
			})

			By("deleting user keys and policies", func() {
				Expect(iamAPI.ListAccessKeysCallCount()).To(Equal(1))
				Expect(iamAPI.ListAccessKeysArgsForCall(0)).ToNot(BeNil())
				Expect(iamAPI.ListAccessKeysArgsForCall(0).UserName).ToNot(BeNil())
				Expect(iamAPI.ListAccessKeysArgsForCall(0).UserName).To(Equal(aws.String(s3ClientConfig.ResourcePrefix + "some-user")))

				Expect(iamAPI.ListAttachedUserPoliciesCallCount()).To(Equal(1))
				Expect(iamAPI.ListAttachedUserPoliciesArgsForCall(0)).ToNot(BeNil())
				Expect(iamAPI.ListAttachedUserPoliciesArgsForCall(0).UserName).ToNot(BeNil())
				Expect(iamAPI.ListAttachedUserPoliciesArgsForCall(0).UserName).To(Equal(aws.String(s3ClientConfig.ResourcePrefix + "some-user")))

				Expect(iamAPI.DeleteAccessKeyCallCount()).To(Equal(1))
				Expect(iamAPI.DeleteAccessKeyArgsForCall(0)).ToNot(BeNil())
				Expect(iamAPI.DeleteAccessKeyArgsForCall(0).AccessKeyId).To(Equal(aws.String("key")))
				Expect(iamAPI.DeleteAccessKeyArgsForCall(0).UserName).To(Equal(aws.String(s3ClientConfig.ResourcePrefix + "some-user")))

				Expect(iamAPI.DetachUserPolicyCallCount()).To(Equal(1))
				Expect(iamAPI.DetachUserPolicyArgsForCall(0)).ToNot(BeNil())
				Expect(iamAPI.DetachUserPolicyArgsForCall(0).PolicyArn).To(Equal(aws.String("foo")))
				Expect(iamAPI.DetachUserPolicyArgsForCall(0).UserName).To(Equal(aws.String(s3ClientConfig.ResourcePrefix + "some-user")))
			})

			By("deleting the user", func() {
				Expect(iamAPI.DeleteUserCallCount()).To(Equal(1))
				Expect(iamAPI.DeleteUserArgsForCall(0)).ToNot(BeNil())
				Expect(iamAPI.DeleteUserArgsForCall(0).UserName).ToNot(BeNil())
				Expect(iamAPI.DeleteUserArgsForCall(0).UserName).To(Equal(aws.String(s3ClientConfig.ResourcePrefix + "some-user")))
			})

			// all calls accounted for
			Expect(s3API.Invocations()).To(HaveLen(2))
			Expect(iamAPI.Invocations()).To(HaveLen(5))
		})

		Context("when getting the bucket policy fails for an unknown reason", func() {
			It("passes through the unrecognized error", func() {
				// Set up fake API
				errGettingPolicy := errors.New("error-getting-policy")
				s3API.GetBucketPolicyReturns(&awsS3.GetBucketPolicyOutput{}, errGettingPolicy)

				err := s3Client.RemoveUserFromBucketAndDeleteUser("some-user", "bucketName")
				Expect(err).To(MatchError(errGettingPolicy))

				By("attempting to get the bucket policy", func() {
					Expect(s3API.GetBucketPolicyCallCount()).To(Equal(1))
					Expect(s3API.GetBucketPolicyArgsForCall(0)).ToNot(BeNil())
					Expect(s3API.GetBucketPolicyArgsForCall(0).Bucket).To(Equal(aws.String(s3ClientConfig.ResourcePrefix + "bucketName")))
				})

				// all calls accounted for
				Expect(s3API.Invocations()).To(HaveLen(1))
				Expect(iamAPI.Invocations()).To(HaveLen(0))
			})
		})

		Context("when deleting the user fails for an unknown reason", func() {
			It("passes through the unrecognized error, having deleted the bucket policy", func() {
				// Set up fake API
				userArn := "arn:aws:iam::account-number:user/s3-broker/" + s3ClientConfig.ResourcePrefix + "some-user"
				s3API.GetBucketPolicyReturns(&awsS3.GetBucketPolicyOutput{
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

				s3API.DeleteBucketPolicyReturns(&awsS3.DeleteBucketPolicyOutput{}, nil)

				errDeletingUser := errors.New("error-deleting-user")
				iamAPI.ListAccessKeysReturns(&iam.ListAccessKeysOutput{}, nil)
				iamAPI.ListAttachedUserPoliciesReturns(&iam.ListAttachedUserPoliciesOutput{}, nil)
				iamAPI.DeleteUserReturns(&iam.DeleteUserOutput{}, errDeletingUser)

				err := s3Client.RemoveUserFromBucketAndDeleteUser("some-user", "bucketName")
				Expect(err).To(MatchError(errDeletingUser))

				By("getting the bucket policy", func() {
					Expect(s3API.GetBucketPolicyCallCount()).To(Equal(1))
					Expect(s3API.GetBucketPolicyArgsForCall(0)).ToNot(BeNil())
					Expect(s3API.GetBucketPolicyArgsForCall(0).Bucket).ToNot(BeNil())
					Expect(s3API.GetBucketPolicyArgsForCall(0).Bucket).To(Equal(aws.String(s3ClientConfig.ResourcePrefix + "bucketName")))
				})

				By("deleting the bucket policy", func() {
					Expect(s3API.DeleteBucketPolicyCallCount()).To(Equal(1))
					Expect(s3API.DeleteBucketPolicyArgsForCall(0)).ToNot(BeNil())
					Expect(s3API.DeleteBucketPolicyArgsForCall(0).Bucket).ToNot(BeNil())
					Expect(s3API.DeleteBucketPolicyArgsForCall(0).Bucket).To(Equal(aws.String(s3ClientConfig.ResourcePrefix + "bucketName")))
				})

				By("checking for user keys and policies", func() {
					Expect(iamAPI.ListAccessKeysCallCount()).To(Equal(1))
					Expect(iamAPI.ListAccessKeysArgsForCall(0)).ToNot(BeNil())
					Expect(iamAPI.ListAccessKeysArgsForCall(0).UserName).ToNot(BeNil())
					Expect(iamAPI.ListAccessKeysArgsForCall(0).UserName).To(Equal(aws.String(s3ClientConfig.ResourcePrefix + "some-user")))

					Expect(iamAPI.ListAttachedUserPoliciesCallCount()).To(Equal(1))
					Expect(iamAPI.ListAttachedUserPoliciesArgsForCall(0)).ToNot(BeNil())
					Expect(iamAPI.ListAttachedUserPoliciesArgsForCall(0).UserName).ToNot(BeNil())
					Expect(iamAPI.ListAttachedUserPoliciesArgsForCall(0).UserName).To(Equal(aws.String(s3ClientConfig.ResourcePrefix + "some-user")))
				})

				By("attempting to delete the user", func() {
					Expect(iamAPI.DeleteUserCallCount()).To(Equal(1))
					Expect(iamAPI.DeleteUserArgsForCall(0)).ToNot(BeNil())
					Expect(iamAPI.DeleteUserArgsForCall(0).UserName).ToNot(BeNil())
					Expect(iamAPI.DeleteUserArgsForCall(0).UserName).To(Equal(aws.String(s3ClientConfig.ResourcePrefix + "some-user")))
				})

				// all calls accounted for
				Expect(s3API.Invocations()).To(HaveLen(2))
				Expect(iamAPI.Invocations()).To(HaveLen(3))
			})
		})

		testUserDoesntExist := func(iamErrorCode string) {
			Context(fmt.Sprintf("the expected bucket policy statement exists but the user doesn't (user %s)", iamErrorCode), func() {
				It("deletes the bucket policy statement and returns no error", func() {
					// Set up fake API
					userArn := "arn:aws:iam::account-number:user/s3-broker/" + s3ClientConfig.ResourcePrefix + "some-user"
					s3API.GetBucketPolicyReturns(&awsS3.GetBucketPolicyOutput{
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

					s3API.DeleteBucketPolicyReturns(&awsS3.DeleteBucketPolicyOutput{}, nil)

					iamAPI.ListAccessKeysReturns(nil, awserr.New(iamErrorCode, "full error message", nil))
					iamAPI.ListAttachedUserPoliciesReturns(nil, awserr.New(iamErrorCode, "full error message", nil))
					iamAPI.DeleteUserReturns(nil, awserr.New(iamErrorCode, "full error message", nil))

					err := s3Client.RemoveUserFromBucketAndDeleteUser("some-user", "bucketName")
					Expect(err).ToNot(HaveOccurred())

					By("getting the bucket policy", func() {
						Expect(s3API.GetBucketPolicyCallCount()).To(Equal(1))
						Expect(s3API.GetBucketPolicyArgsForCall(0)).ToNot(BeNil())
						Expect(s3API.GetBucketPolicyArgsForCall(0).Bucket).ToNot(BeNil())
						Expect(s3API.GetBucketPolicyArgsForCall(0).Bucket).To(Equal(aws.String(s3ClientConfig.ResourcePrefix + "bucketName")))
					})

					By("deleting the bucket policy", func() {
						Expect(s3API.DeleteBucketPolicyCallCount()).To(Equal(1))
						Expect(s3API.DeleteBucketPolicyArgsForCall(0)).ToNot(BeNil())
						Expect(s3API.DeleteBucketPolicyArgsForCall(0).Bucket).ToNot(BeNil())
						Expect(s3API.DeleteBucketPolicyArgsForCall(0).Bucket).To(Equal(aws.String(s3ClientConfig.ResourcePrefix + "bucketName")))
					})

					By("checking for user keys and policies", func() {
						Expect(iamAPI.ListAccessKeysCallCount()).To(Equal(1))
						Expect(iamAPI.ListAccessKeysArgsForCall(0)).ToNot(BeNil())
						Expect(iamAPI.ListAccessKeysArgsForCall(0).UserName).To(Equal(aws.String("test-bucket-prefix-some-user")))

						Expect(iamAPI.ListAttachedUserPoliciesCallCount()).To(Equal(1))
						Expect(iamAPI.ListAttachedUserPoliciesArgsForCall(0)).ToNot(BeNil())
						Expect(iamAPI.ListAttachedUserPoliciesArgsForCall(0).UserName).To(Equal(aws.String("test-bucket-prefix-some-user")))
					})

					By("attempting to delete the user", func() {
						Expect(iamAPI.DeleteUserCallCount()).To(Equal(1))
						Expect(iamAPI.DeleteUserArgsForCall(0)).ToNot(BeNil())
						Expect(iamAPI.DeleteUserArgsForCall(0).UserName).To(Equal(aws.String("test-bucket-prefix-some-user")))
					})

					// all calls accounted for
					Expect(s3API.Invocations()).To(HaveLen(2))
					Expect(iamAPI.Invocations()).To(HaveLen(3))
				})
			})
		}
		testUserDoesntExist(iam.ErrCodeNoSuchEntityException)
		testUserDoesntExist("AccessDenied")

		Context("when the expected user and policy statement don't exist", func() {
			It("returns ErrNoSuchResources", func() {
				// Set up fake API
				s3API.GetBucketPolicyReturns(&awsS3.GetBucketPolicyOutput{
					Policy: aws.String(`
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
										"AWS": "some-other-arn"
									}
								}
							]
						}`),
				}, nil)
				iamAPI.ListAccessKeysReturns(nil, awserr.New(iam.ErrCodeNoSuchEntityException, "full error message", nil))
				iamAPI.ListAttachedUserPoliciesReturns(nil, awserr.New(iam.ErrCodeNoSuchEntityException, "full error message", nil))
				iamAPI.DeleteUserReturns(nil, awserr.New(iam.ErrCodeNoSuchEntityException, "full error message", nil))

				err := s3Client.RemoveUserFromBucketAndDeleteUser("some-user", "bucketName")
				Expect(err).To(MatchError(s3.ErrNoSuchResources))

				By("getting the bucket policy", func() {
					Expect(s3API.GetBucketPolicyCallCount()).To(Equal(1))
					Expect(s3API.GetBucketPolicyArgsForCall(0)).ToNot(BeNil())
					Expect(s3API.GetBucketPolicyArgsForCall(0).Bucket).To(Equal(aws.String(s3ClientConfig.ResourcePrefix + "bucketName")))
				})

				By("checking for user keys and policies", func() {
					Expect(iamAPI.ListAccessKeysCallCount()).To(Equal(1))
					Expect(iamAPI.ListAccessKeysArgsForCall(0)).ToNot(BeNil())
					Expect(iamAPI.ListAccessKeysArgsForCall(0).UserName).To(Equal(aws.String("test-bucket-prefix-some-user")))

					Expect(iamAPI.ListAttachedUserPoliciesCallCount()).To(Equal(1))
					Expect(iamAPI.ListAttachedUserPoliciesArgsForCall(0)).ToNot(BeNil())
					Expect(iamAPI.ListAttachedUserPoliciesArgsForCall(0).UserName).To(Equal(aws.String("test-bucket-prefix-some-user")))
				})

				By("attempting to delete the user", func() {
					Expect(iamAPI.DeleteUserCallCount()).To(Equal(1))
					Expect(iamAPI.DeleteUserArgsForCall(0)).ToNot(BeNil())
					Expect(iamAPI.DeleteUserArgsForCall(0).UserName).To(Equal(aws.String("test-bucket-prefix-some-user")))
				})

				// all calls accounted for
				Expect(s3API.Invocations()).To(HaveLen(1))
				Expect(iamAPI.Invocations()).To(HaveLen(3))
			})
		})

		testNothingExists := func(iamErrorCode string) {
			Context(fmt.Sprintf("when the expected user and the bucket policy don't exist (user %s)", iamErrorCode), func() {
				It("returns ErrNoSuchResources", func() {
					// Set up fake API
					s3API.GetBucketPolicyReturns(nil, awserr.New("NoSuchBucketPolicy", "full error message", nil))
					iamAPI.ListAccessKeysReturns(nil, awserr.New(iamErrorCode, "full error message", nil))
					iamAPI.ListAttachedUserPoliciesReturns(nil, awserr.New(iamErrorCode, "full error message", nil))
					iamAPI.DeleteUserReturns(&iam.DeleteUserOutput{}, awserr.New(iamErrorCode, "full error message", nil))

					err := s3Client.RemoveUserFromBucketAndDeleteUser("some-user", "bucketName")
					Expect(err).To(MatchError(s3.ErrNoSuchResources))

					By("attempting to get the bucket policy", func() {
						Expect(s3API.GetBucketPolicyCallCount()).To(Equal(1))
						Expect(s3API.GetBucketPolicyArgsForCall(0)).ToNot(BeNil())
						Expect(s3API.GetBucketPolicyArgsForCall(0).Bucket).To(Equal(aws.String(s3ClientConfig.ResourcePrefix + "bucketName")))
					})

					By("checking for user keys and policies", func() {
						Expect(iamAPI.ListAccessKeysCallCount()).To(Equal(1))
						Expect(iamAPI.ListAccessKeysArgsForCall(0)).ToNot(BeNil())
						Expect(iamAPI.ListAccessKeysArgsForCall(0).UserName).To(Equal(aws.String("test-bucket-prefix-some-user")))

						Expect(iamAPI.ListAttachedUserPoliciesCallCount()).To(Equal(1))
						Expect(iamAPI.ListAttachedUserPoliciesArgsForCall(0)).ToNot(BeNil())
						Expect(iamAPI.ListAttachedUserPoliciesArgsForCall(0).UserName).To(Equal(aws.String("test-bucket-prefix-some-user")))
					})

					By("attempting to delete the user", func() {
						Expect(iamAPI.DeleteUserCallCount()).To(Equal(1))
						Expect(iamAPI.DeleteUserArgsForCall(0)).ToNot(BeNil())
						Expect(iamAPI.DeleteUserArgsForCall(0).UserName).To(Equal(aws.String("test-bucket-prefix-some-user")))
					})

					// all calls accounted for
					Expect(s3API.Invocations()).To(HaveLen(1))
					Expect(iamAPI.Invocations()).To(HaveLen(3))
				})
			})
		}
		testNothingExists(iam.ErrCodeNoSuchEntityException)
		testNothingExists("AccessDenied")

		Context("when the bucket policy statement doesn't exist but the user does", func() {
			It("still deletes the user and returns no error", func() {
				// Set up fake API
				s3API.GetBucketPolicyReturns(nil, awserr.New("NoSuchBucketPolicy", "full error message", nil))
				iamAPI.ListAccessKeysReturns(&iam.ListAccessKeysOutput{}, nil)
				iamAPI.ListAttachedUserPoliciesReturns(&iam.ListAttachedUserPoliciesOutput{
					AttachedPolicies: []*iam.AttachedPolicy{
						{
							PolicyArn:  aws.String("foo"),
							PolicyName: aws.String("bar"),
						},
					},
				}, nil)
				iamAPI.DeleteAccessKeyReturns(nil, nil)

				err := s3Client.RemoveUserFromBucketAndDeleteUser("some-user", "bucketName")
				Expect(err).ToNot(HaveOccurred())

				By("attempting to get the bucket policy", func() {
					Expect(s3API.GetBucketPolicyCallCount()).To(Equal(1))
					Expect(s3API.GetBucketPolicyArgsForCall(0)).ToNot(BeNil())
					Expect(s3API.GetBucketPolicyArgsForCall(0).Bucket).To(Equal(aws.String(s3ClientConfig.ResourcePrefix + "bucketName")))
				})

				By("deleting user keys and policies", func() {
					Expect(iamAPI.ListAccessKeysCallCount()).To(Equal(1))
					Expect(iamAPI.ListAccessKeysArgsForCall(0)).ToNot(BeNil())
					Expect(iamAPI.ListAccessKeysArgsForCall(0).UserName).To(Equal(aws.String("test-bucket-prefix-some-user")))

					Expect(iamAPI.ListAttachedUserPoliciesCallCount()).To(Equal(1))
					Expect(iamAPI.ListAttachedUserPoliciesArgsForCall(0)).ToNot(BeNil())
					Expect(iamAPI.ListAttachedUserPoliciesArgsForCall(0).UserName).To(Equal(aws.String("test-bucket-prefix-some-user")))

					Expect(iamAPI.DetachUserPolicyCallCount()).To(Equal(1))
					Expect(iamAPI.DetachUserPolicyArgsForCall(0)).ToNot(BeNil())
					Expect(iamAPI.DetachUserPolicyArgsForCall(0).PolicyArn).To(Equal(aws.String("foo")))
					Expect(iamAPI.DetachUserPolicyArgsForCall(0).UserName).To(Equal(aws.String(s3ClientConfig.ResourcePrefix + "some-user")))
				})

				By("deleting the user", func() {
					Expect(iamAPI.DeleteUserCallCount()).To(Equal(1))
					Expect(iamAPI.DeleteUserArgsForCall(0)).ToNot(BeNil())
					Expect(iamAPI.DeleteUserArgsForCall(0).UserName).ToNot(BeNil())
					Expect(iamAPI.DeleteUserArgsForCall(0).UserName).To(Equal(aws.String(s3ClientConfig.ResourcePrefix + "some-user")))
				})

				// all calls accounted for
				Expect(s3API.Invocations()).To(HaveLen(1))
				Expect(iamAPI.Invocations()).To(HaveLen(4))
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
