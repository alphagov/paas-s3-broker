package provider_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"context"
	"errors"
	provideriface "github.com/alphagov/paas-go/provider"
	"github.com/alphagov/paas-s3-broker/provider"
	fakeClient "github.com/alphagov/paas-s3-broker/s3/fakes"
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
			fakeClientErr := errors.New("error provisioning")
			fakeS3Client.CreateBucketReturns(fakeClientErr)

			_, _, _, err := s3Provider.Provision(context.Background(), provisionData)
			Expect(err).To(MatchError(fakeClientErr))
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

		It("errors if the client errors", func() {
			deprovisionData := provideriface.DeprovisionData{
				InstanceID: "09E1993E-62E2-4040-ADF2-4D3EC741EFE6",
			}
			fakeClientErr := errors.New("error deprovisioning")
			fakeS3Client.DeleteBucketReturns(fakeClientErr)

			_, _, err := s3Provider.Deprovision(context.Background(), deprovisionData)
			Expect(err).To(MatchError(fakeClientErr))
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
