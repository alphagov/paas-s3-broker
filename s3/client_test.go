package s3_test

import (
	"errors"
	"fmt"
	"time"

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
		s3API    *fakeClient.FakeS3API
		iamAPI   *fakeClient.FakeIAMAPI
		s3Client s3.S3Client
	)

	BeforeEach(func() {
		s3API = &fakeClient.FakeS3API{}
		iamAPI = &fakeClient.FakeIAMAPI{}
		s3Client = s3.S3Client{
			Timeout: 2 * time.Second,
			S3:      s3API,
			IAM:     iamAPI,
		}
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
				Policy: aws.String(`{}`),
			}, nil)

			bucketCredentials, err := s3Client.AddUserToBucket("username", "bucketName")
			Expect(err).NotTo(HaveOccurred())

			By("creating a user")
			Expect(iamAPI.CreateUserCallCount()).To(Equal(1))

			By("creating access keys for the user")
			Expect(iamAPI.CreateAccessKeyCallCount()).To(Equal(1))

			By("getting the bucket policy")
			Expect(s3API.GetBucketPolicyCallCount()).To(Equal(1))

			By("putting the updated policy")
			Expect(s3API.PutBucketPolicyCallCount()).To(Equal(1))

			By("returning the bucket credentials")
			Expect(bucketCredentials).To(Equal(s3.BucketCredentials{
				BucketName:         "bucketName",
				AWSAccessKeyID:     "access-key-id",
				AWSSecretAccessKey: "secret-access-key",
			}))
		})

		Context("when creating an access key fails", func() {
			It("deletes the user", func() {
				// Set up fake API
				iamAPI.CreateUserReturnsOnCall(0, &iam.CreateUserOutput{
					User: &iam.User{},
				}, nil)
				iamAPI.CreateAccessKeyReturnsOnCall(0, &iam.CreateAccessKeyOutput{}, errors.New("some-error"))

				_, err := s3Client.AddUserToBucket("username", "bucketName")
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

				bucketCredentials, err := s3Client.AddUserToBucket("username", "bucketName")
				Expect(err).NotTo(HaveOccurred())
				Expect(bucketCredentials).To(Equal(s3.BucketCredentials{
					BucketName:         "bucketName",
					AWSAccessKeyID:     "access-key-id",
					AWSSecretAccessKey: "secret-access-key",
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

				_, err := s3Client.AddUserToBucket("username", "bucketName")
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

				_, err := s3Client.AddUserToBucket("username", "bucketName")
				Expect(err).To(HaveOccurred())
				Expect(iamAPI.DeleteUserCallCount()).To(Equal(1))
				Expect(iamAPI.DeleteAccessKeyCallCount()).To(Equal(1))
			})
		})

		Describe("AddUserToBucketPolicy", func() {
			It("adds the user to an empty policy", func() {
				userArn := "arn:aws:iam::acount-number:user/s3-broker/some-user"
				updatedPolicy, err := s3Client.AddUserToBucketPolicy(
					userArn,
					"bucketName",
					"",
				)
				Expect(err).NotTo(HaveOccurred())

				expectedPolicy := s3.PolicyDocument{
					Version: "2012-10-17",
					Statement: []s3.Statement{
						{
							Effect:   "Allow",
							Action:   []string{"s3:DeleteObject", "s3:GetObject", "s3:PutObject"},
							Resource: "arn:aws:s3:::bucketName/*",
							Principal: s3.Principal{
								AWS: userArn,
							},
						},
					},
				}
				Expect(updatedPolicy).To(Equal(expectedPolicy))
			})

			It("errors if the policy can't be unmarshalled", func() {
				_, err := s3Client.AddUserToBucketPolicy(
					"userArn",
					"bucketName",
					"this is clearly not JSON",
				)
				Expect(err).To(HaveOccurred())
			})

			Context("when adding users to an existing policy", func() {
				It("converts the principal string into an array", func() {
					newUserArn := "arn:aws:iam::acount-number:user/s3-broker/new-user"
					originalUserArn := "arn:aws:iam::acount-number:user/s3-broker/original-user"
					updatedPolicy, err := s3Client.AddUserToBucketPolicy(
						newUserArn,
						"bucketName",
						fmt.Sprintf(`
						{
							"Statement": [
								{
									"Action": [
										"s3:DeleteObject",
										"s3:GetObject",
										"s3:PutObject"
									],
									"Effect": "Allow",
									"Resource": "arn:aws:s3:::gds-paas-s3-broker-bucketName/*",
									"Principal": {
										"AWS": "%s"
									}
								}
							]
						}
					`, originalUserArn),
					)
					Expect(err).NotTo(HaveOccurred())

					principal := updatedPolicy.Statement[0].Principal.AWS
					concretePrincipal := principal.([]string)
					Expect(concretePrincipal).To(BeAssignableToTypeOf([]string{}))
					Expect(len(concretePrincipal)).To(Equal(2))
					Expect(concretePrincipal).To(ContainElement(newUserArn))
					Expect(concretePrincipal).To(ContainElement(originalUserArn))
				})

				It("appends to an existing principal array", func() {
					newUserArn := "arn:aws:iam::acount-number:user/s3-broker/new-user"
					originalUserArn1 := "arn:aws:iam::acount-number:user/s3-broker/original-user1"
					originalUserArn2 := "arn:aws:iam::acount-number:user/s3-broker/original-user2"
					updatedPolicy, err := s3Client.AddUserToBucketPolicy(
						newUserArn,
						"bucketName",
						fmt.Sprintf(`
						{
							"Statement": [
								{
									"Action": [
										"s3:DeleteObject",
										"s3:GetObject",
										"s3:PutObject"
									],
									"Effect": "Allow",
									"Resource": "arn:aws:s3:::gds-paas-s3-broker-bucketName/*",
									"Principal": {
										"AWS": [
											"%s",
											"%s"
										]
									}
								}
							]
						}
					`, originalUserArn1, originalUserArn2),
					)
					Expect(err).NotTo(HaveOccurred())

					principal := updatedPolicy.Statement[0].Principal.AWS
					concretePrincipal := principal.([]string)
					Expect(concretePrincipal).To(BeAssignableToTypeOf([]string{}))
					Expect(len(concretePrincipal)).To(Equal(3))
					Expect(concretePrincipal).To(ContainElement(newUserArn))
					Expect(concretePrincipal).To(ContainElement(originalUserArn1))
					Expect(concretePrincipal).To(ContainElement(originalUserArn2))
				})
			})
		})
	})

	Describe("RemoveUserFromBucket", func() {
		It("manages the user and bucket policy", func() {
			// Set up fake API
			userArn := "arn:aws:iam::acount-number:user/s3-broker/some-user"
			s3API.GetBucketPolicyReturnsOnCall(0, &awsS3.GetBucketPolicyOutput{
				Policy: aws.String(fmt.Sprintf(`
					{
						"Statement": [
							{
								"Action": [
									"s3:DeleteObject",
									"s3:GetObject",
									"s3:PutObject"
								],
								"Effect": "Allow",
								"Resource": "arn:aws:s3:::gds-paas-s3-broker-bucketName/*",
								"Principal": {
									"AWS": "%s"
								}
							}
						]
					}`, userArn)),
			}, nil)
			s3API.DeleteBucketPolicyReturnsOnCall(0, nil, nil)
			iamAPI.ListAccessKeysReturnsOnCall(0, &iam.ListAccessKeysOutput{
				AccessKeyMetadata: []*iam.AccessKeyMetadata{{AccessKeyId: aws.String("key")}},
			}, nil)
			iamAPI.DeleteAccessKeyReturnsOnCall(0, nil, nil)

			err := s3Client.RemoveUserFromBucket("some-user", "bucketName")
			Expect(err).NotTo(HaveOccurred())

			By("getting the bucket policy")
			Expect(s3API.GetBucketPolicyCallCount()).To(Equal(1))

			By("deleting the policy")
			Expect(s3API.DeleteBucketPolicyCallCount()).To(Equal(1))

			By("deleting the user and keys")
			Expect(iamAPI.DeleteUserCallCount()).To(Equal(1))
			Expect(iamAPI.DeleteAccessKeyCallCount()).To(Equal(1))
		})

		Context("when getting the bucket policy fails", func() {
			It("returns an error", func() {
				// Set up fake API
				errGettingPolicy := errors.New("error-getting-policy")
				s3API.GetBucketPolicyReturnsOnCall(0, &awsS3.GetBucketPolicyOutput{}, errGettingPolicy)

				err := s3Client.RemoveUserFromBucket("username", "bucketName")
				Expect(err).To(MatchError(errGettingPolicy))
			})
		})

		Context("when deleting the user fails", func() {
			It("returns an error", func() {
				// Set up fake API
				userArn := "arn:aws:iam::acount-number:user/s3-broker/some-user"
				s3API.GetBucketPolicyReturnsOnCall(0, &awsS3.GetBucketPolicyOutput{
					Policy: aws.String(fmt.Sprintf(`
					{
						"Statement": [
							{
								"Action": [
									"s3:DeleteObject",
									"s3:GetObject",
									"s3:PutObject"
								],
								"Effect": "Allow",
								"Resource": "arn:aws:s3:::gds-paas-s3-broker-bucketName/*",
								"Principal": {
									"AWS": "%s"
								}
							}
						]
					}`, userArn)),
				}, nil)

				s3API.DeleteBucketPolicyReturns(&awsS3.DeleteBucketPolicyOutput{}, nil)

				errDeletingUser := errors.New("error-deleting-user")
				iamAPI.DeleteUserReturnsOnCall(0, &iam.DeleteUserOutput{}, errDeletingUser)

				err := s3Client.RemoveUserFromBucket("some-user", "bucketName")
				Expect(err).To(MatchError(errDeletingUser))
			})
		})

		Describe("RemoveUserFromBucketPolicy", func() {
			It("removes the policy if it only has a single principal", func() {
				// Set up fake API
				s3API.DeleteBucketPolicyReturnsOnCall(0, &awsS3.DeleteBucketPolicyOutput{}, nil)

				userArn := "arn:aws:iam::acount-number:user/s3-broker/some-user"
				_, _ = s3Client.RemoveUserFromBucketPolicy(
					userArn,
					"bucketName",
					fmt.Sprintf(`
						{
							"Statement": [
								{
									"Action": [
										"s3:DeleteObject",
										"s3:GetObject",
										"s3:PutObject"
									],
									"Effect": "Allow",
									"Resource": "arn:aws:s3:::gds-paas-s3-broker-bucketName/*",
									"Principal": {
										"AWS": "%s"
									}
								}
							]
						}
					`, userArn),
				)
				Expect(s3API.DeleteBucketPolicyCallCount()).To(Equal(1))
			})

			It("remove the user from the list of principles, preserving any other users", func() {
				// Set up fake API
				s3API.PutBucketPolicyReturnsOnCall(0, &awsS3.PutBucketPolicyOutput{}, nil)

				user1Name := "user1"
				user1Arn := "arn:aws:iam::acount-number:user/s3-broker/user1"
				user2Arn := "arn:aws:iam::acount-number:user/s3-broker/user2"
				updatedPolicy, err := s3Client.RemoveUserFromBucketPolicy(
					user1Name,
					"bucketName",
					fmt.Sprintf(`
						{
							"Statement": [
								{
									"Action": [
										"s3:DeleteObject",
										"s3:GetObject",
										"s3:PutObject"
									],
									"Effect": "Allow",
									"Resource": "arn:aws:s3:::gds-paas-s3-broker-bucketName/*",
									"Principal": {
										"AWS": [
											"%s",
											"%s"
										]
									}
								}
							]
						}
					`, user1Arn, user2Arn),
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(s3API.PutBucketPolicyCallCount()).To(Equal(1))

				principal := updatedPolicy.Statement[0].Principal.AWS
				concretePrincipal := principal.([]string)
				Expect(concretePrincipal).To(BeAssignableToTypeOf([]string{}))
				Expect(len(concretePrincipal)).To(Equal(1))
				Expect(concretePrincipal).To(ContainElement(user2Arn))
			})
		})
	})
})
