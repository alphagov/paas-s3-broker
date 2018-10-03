package provider_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"context"
	"errors"

	provideriface "github.com/alphagov/paas-go/provider"
	"github.com/alphagov/paas-s3-broker/provider"
	"github.com/alphagov/paas-s3-broker/s3"
	fakeClient "github.com/alphagov/paas-s3-broker/s3/fakes"
	"github.com/pivotal-cf/brokerapi"
)

var _ = Describe("Provider", func() {
	var (
		fakeS3Client *fakeClient.FakeClient
		s3Provider   *provider.S3Provider
	)

	BeforeEach(func() {
		fakeS3Client = &fakeClient.FakeClient{}

		var err error
		s3Provider, err = provider.NewS3Provider([]byte(`{}`))
		Expect(err).NotTo(HaveOccurred())
		s3Provider.Client = fakeS3Client
	})

	Describe("Provision", func() {
		It("passes the correct parameters to the client", func() {
			provisionData := provideriface.ProvisionData{
				InstanceID: "09E1993E-62E2-4040-ADF2-4D3EC741EFE6",
			}
			fakeS3Client.CreateBucketReturns(nil)

			_, _, _, err := s3Provider.Provision(context.Background(), provisionData)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeS3Client.CreateBucketArgsForCall(0)).To(Equal(provisionData.InstanceID))
		})

		It("errors if the client errors", func() {
			provisionData := provideriface.ProvisionData{
				InstanceID: "09E1993E-62E2-4040-ADF2-4D3EC741EFE6",
			}
			errProvisioning := errors.New("error provisioning")
			fakeS3Client.CreateBucketReturns(errProvisioning)

			_, _, _, err := s3Provider.Provision(context.Background(), provisionData)
			Expect(err).To(MatchError(errProvisioning))
		})
	})

	Describe("Deprovision", func() {
		It("passes the correct parameters to the client", func() {
			deprovisionData := provideriface.DeprovisionData{
				InstanceID: "09E1993E-62E2-4040-ADF2-4D3EC741EFE6",
			}
			fakeS3Client.DeleteBucketReturns(nil)

			_, _, err := s3Provider.Deprovision(context.Background(), deprovisionData)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeS3Client.DeleteBucketArgsForCall(0)).To(Equal(deprovisionData.InstanceID))
		})

		It("returns a specific error if the bucket does not exist", func() {
			fakeS3Client.DeleteBucketReturns(errors.New("NoSuchBucket: The specified bucket does not exist"))
			_, _, err := s3Provider.Deprovision(context.Background(), provideriface.DeprovisionData{})
			Expect(err).To(MatchError(brokerapi.ErrInstanceDoesNotExist))
		})

		It("errors if the client errors", func() {
			deprovisionData := provideriface.DeprovisionData{
				InstanceID: "09E1993E-62E2-4040-ADF2-4D3EC741EFE6",
			}
			errDeprovisioning := errors.New("error deprovisioning")
			fakeS3Client.DeleteBucketReturns(errDeprovisioning)

			_, _, err := s3Provider.Deprovision(context.Background(), deprovisionData)
			Expect(err).To(MatchError(errDeprovisioning))
		})
	})

	Describe("Bind", func() {
		It("passes the correct parameters to the client", func() {
			instanceID := "09E1993E-62E2-4040-ADF2-4D3EC741EFE6"
			bindingID := "D26EA3FB-AA78-451C-9ED0-233935ED388F"

			bindData := provideriface.BindData{
				InstanceID: instanceID,
				BindingID:  bindingID,
			}
			returnedBucketCredentials := s3.BucketCredentials{
				BucketName:         s3Provider.Config.BucketPrefix + "bucketName",
				AWSAccessKeyID:     "aws-access-key-id",
				AWSSecretAccessKey: "aws-secret-access-key",
			}
			fakeS3Client.AddUserToBucketReturns(returnedBucketCredentials, nil)

			binding, err := s3Provider.Bind(context.Background(), bindData)
			Expect(err).NotTo(HaveOccurred())
			actualUsername, actualBucketName := fakeS3Client.AddUserToBucketArgsForCall(0)
			Expect(actualUsername).To(Equal(bindingID))
			Expect(actualBucketName).To(Equal(instanceID))

			Expect(binding.Credentials).To(Equal(returnedBucketCredentials))
		})

		It("errors when adding the user errors", func() {
			bindData := provideriface.BindData{}
			errAddingUser := errors.New("error-adding-user")
			fakeS3Client.AddUserToBucketReturns(s3.BucketCredentials{}, errAddingUser)

			_, err := s3Provider.Bind(context.Background(), bindData)
			Expect(err).To(MatchError(errAddingUser))
		})
	})

	Describe("Update", func() {
		It("does not support updating a bucket", func() {
			updateData := provideriface.UpdateData{
				InstanceID: "09E1993E-62E2-4040-ADF2-4D3EC741EFE6",
			}

			_, _, err := s3Provider.Update(context.Background(), updateData)
			Expect(err).To(MatchError(provider.ErrUpdateNotSupported))
		})
	})
})
