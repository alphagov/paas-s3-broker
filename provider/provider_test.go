package provider_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"context"
	"errors"

	"github.com/alphagov/paas-s3-broker/provider"
	"github.com/alphagov/paas-s3-broker/s3"
	fakeClient "github.com/alphagov/paas-s3-broker/s3/fakes"
	provideriface "github.com/alphagov/paas-service-broker-base/provider"
	"github.com/pivotal-cf/brokerapi/v10/domain"
	"github.com/pivotal-cf/brokerapi/v10/domain/apiresponses"
)

var _ = Describe("Provider", func() {
	var (
		fakeS3Client *fakeClient.FakeClient
		s3Provider   *provider.S3Provider
	)

	BeforeEach(func() {
		fakeS3Client = &fakeClient.FakeClient{}
		s3Provider = provider.NewS3Provider(fakeS3Client)
	})

	Describe("Provision", func() {
		It("passes the correct parameters to the client", func() {
			provisionData := provideriface.ProvisionData{
				InstanceID: "09E1993E-62E2-4040-ADF2-4D3EC741EFE6",
			}
			fakeS3Client.CreateBucketReturns(nil)

			_, err := s3Provider.Provision(context.Background(), provisionData)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeS3Client.CreateBucketArgsForCall(0)).To(Equal(provisionData))
		})

		It("errors if the client errors", func() {
			provisionData := provideriface.ProvisionData{
				InstanceID: "09E1993E-62E2-4040-ADF2-4D3EC741EFE6",
			}
			errProvisioning := errors.New("error provisioning")
			fakeS3Client.CreateBucketReturns(errProvisioning)

			_, err := s3Provider.Provision(context.Background(), provisionData)
			Expect(err).To(MatchError(errProvisioning))
		})
	})

	Describe("Deprovision", func() {
		It("passes the correct parameters to the client", func() {
			deprovisionData := provideriface.DeprovisionData{
				InstanceID: "09E1993E-62E2-4040-ADF2-4D3EC741EFE6",
			}
			fakeS3Client.DeleteBucketReturns(nil)

			_, err := s3Provider.Deprovision(context.Background(), deprovisionData)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeS3Client.DeleteBucketArgsForCall(0)).To(Equal(deprovisionData.InstanceID))
		})

		It("returns a specific error if the bucket does not exist", func() {
			fakeS3Client.DeleteBucketReturns(s3.ErrNoSuchResources)
			_, err := s3Provider.Deprovision(context.Background(), provideriface.DeprovisionData{})
			Expect(err).To(MatchError(apiresponses.ErrInstanceDoesNotExist))
		})

		It("errors if the client errors", func() {
			deprovisionData := provideriface.DeprovisionData{
				InstanceID: "09E1993E-62E2-4040-ADF2-4D3EC741EFE6",
			}
			errDeprovisioning := errors.New("error deprovisioning")
			fakeS3Client.DeleteBucketReturns(errDeprovisioning)

			_, err := s3Provider.Deprovision(context.Background(), deprovisionData)
			Expect(err).To(MatchError(errDeprovisioning))
		})
	})

	Describe("Bind", func() {
		It("passes the correct parameters to the client", func() {
			instanceID := "09E1993E-62E2-4040-ADF2-4D3EC741EFE6"
			bindingID := "D26EA3FB-AA78-451C-9ED0-233935ED388F"
			region := "eu-west-2"

			bindData := provideriface.BindData{
				InstanceID: instanceID,
				BindingID:  bindingID,
				Details: domain.BindDetails{
					RawParameters: json.RawMessage(`{"permissions":"read-only"}`),
				},
			}
			returnedBucketCredentials := s3.BucketCredentials{
				BucketName:         "test-paas-s3-broker-bucketName",
				AWSAccessKeyID:     "aws-access-key-id",
				AWSSecretAccessKey: "aws-secret-access-key",
				AWSRegion:          region,
			}
			fakeS3Client.AddUserToBucketReturns(returnedBucketCredentials, nil)

			binding, err := s3Provider.Bind(context.Background(), bindData)
			Expect(err).NotTo(HaveOccurred())
			actualBindData := fakeS3Client.AddUserToBucketArgsForCall(0)
			Expect(actualBindData).To(Equal(bindData))

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

	Describe("Unbind", func() {
		It("passes the correct parameters to the client", func() {
			instanceID := "09E1993E-62E2-4040-ADF2-4D3EC741EFE6"
			bindingID := "D26EA3FB-AA78-451C-9ED0-233935ED388F"

			unbindData := provideriface.UnbindData{
				InstanceID: instanceID,
				BindingID:  bindingID,
			}
			fakeS3Client.RemoveUserFromBucketAndDeleteUserReturns(nil)

			_, err := s3Provider.Unbind(context.Background(), unbindData)
			Expect(err).NotTo(HaveOccurred())
			actualUsername, actualBucketName := fakeS3Client.RemoveUserFromBucketAndDeleteUserArgsForCall(0)
			Expect(actualUsername).To(Equal(bindingID))
			Expect(actualBucketName).To(Equal(instanceID))
		})

		It("passes through the error when removing the user returns an unexpected error", func() {
			unbindData := provideriface.UnbindData{}
			errRemovingUser := errors.New("error-removing-user")
			fakeS3Client.RemoveUserFromBucketAndDeleteUserReturns(errRemovingUser)

			_, err := s3Provider.Unbind(context.Background(), unbindData)
			Expect(err).To(MatchError(errRemovingUser))
		})

		It("returns ErrBindingDoesNotExist when removing the user returns ErrNoSuchResources", func() {
			unbindData := provideriface.UnbindData{}
			fakeS3Client.RemoveUserFromBucketAndDeleteUserReturns(s3.ErrNoSuchResources)

			_, err := s3Provider.Unbind(context.Background(), unbindData)
			Expect(err).To(MatchError(apiresponses.ErrBindingDoesNotExist))
		})
	})

	Describe("Update", func() {
		It("does not support updating a bucket", func() {
			updateData := provideriface.UpdateData{
				InstanceID: "09E1993E-62E2-4040-ADF2-4D3EC741EFE6",
			}

			_, err := s3Provider.Update(context.Background(), updateData)
			Expect(err).To(MatchError(provider.ErrUpdateNotSupported))
		})
	})

	Describe("LastOperation", func() {
		It("returns success unconditionally", func() {
			state, err := s3Provider.LastOperation(context.Background(), provideriface.LastOperationData{})
			Expect(err).NotTo(HaveOccurred())
			Expect(state.Description).To(Equal("Last operation polling not required. All operations are synchronous."))
			Expect(state.State).To(Equal(domain.Succeeded))
		})
	})
})
