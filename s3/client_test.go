package s3_test

import (
	"errors"
	"fmt"
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
			S3:  s3API,
			IAM: iamAPI,
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

			By("Getting the bucket policy")
			Expect(s3API.GetBucketPolicyCallCount()).To(Equal(1))

			By("Putting the updated policy")
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

		Context("when getting the bucket policy fails", func() {
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
				s3API.GetBucketPolicyReturnsOnCall(0, &awsS3.GetBucketPolicyOutput{}, errors.New("some-error"))

				bucketCredentials, err := s3Client.AddUserToBucket("username", "bucketName")
				Expect(err).NotTo(HaveOccurred())
				Expect(bucketCredentials).To(Equal(s3.BucketCredentials{
					BucketName:         "bucketName",
					AWSAccessKeyID:     "access-key-id",
					AWSSecretAccessKey: "secret-access-key",
				}))
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
				s3API.PutBucketPolicyReturnsOnCall(0, &awsS3.PutBucketPolicyOutput{}, errors.New("some-error"))

				_, err := s3Client.AddUserToBucket("username", "bucketName")
				Expect(err).To(HaveOccurred())
				Expect(iamAPI.DeleteUserCallCount()).To(Equal(1))
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
})
