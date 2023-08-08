package provider

import (
	"context"
	"errors"

	"github.com/alphagov/paas-s3-broker/s3"
	provideriface "github.com/alphagov/paas-service-broker-base/provider"
	"github.com/pivotal-cf/brokerapi"
	"github.com/pivotal-cf/brokerapi/domain"
)

type S3Provider struct {
	client s3.Client
}

func NewS3Provider(s3Client s3.Client) *S3Provider {
	return &S3Provider{
		client: s3Client,
	}
}

func (s *S3Provider) Provision(ctx context.Context, provisionData provideriface.ProvisionData) (
	*domain.ProvisionedServiceSpec, error) {

	err := s.client.CreateBucket(provisionData)

	return &domain.ProvisionedServiceSpec{IsAsync: false}, err
}

func (s *S3Provider) Deprovision(ctx context.Context, deprovisionData provideriface.DeprovisionData) (
	*domain.DeprovisionServiceSpec, error) {

	err := s.client.DeleteBucket(deprovisionData.InstanceID)
	if err == s3.ErrNoSuchResources {
		return &domain.DeprovisionServiceSpec{IsAsync: false}, brokerapi.ErrInstanceDoesNotExist
	}
	return &domain.DeprovisionServiceSpec{IsAsync: false}, err
}

func (s *S3Provider) Bind(ctx context.Context, bindData provideriface.BindData) (
	*domain.Binding, error) {
	// binding brokerapi.Binding, err error) {

	bucketCredentials, err := s.client.AddUserToBucket(bindData)
	if err != nil {
		return &domain.Binding{}, err
	}

	return &domain.Binding{
		IsAsync:     false,
		Credentials: bucketCredentials,
	}, nil
}

func (s *S3Provider) Unbind(ctx context.Context, unbindData provideriface.UnbindData) (
	*domain.UnbindSpec, error) {

	err := s.client.RemoveUserFromBucketAndDeleteUser(unbindData.BindingID, unbindData.InstanceID)
	if err != nil {
		if err == s3.ErrNoSuchResources {
			return &domain.UnbindSpec{}, brokerapi.ErrBindingDoesNotExist
		}
		return &domain.UnbindSpec{}, err
	}
	return &domain.UnbindSpec{
		IsAsync: false,
	}, nil
}

var ErrUpdateNotSupported = errors.New("Updating the S3 bucket is currently not supported")

func (s *S3Provider) Update(ctx context.Context, updateData provideriface.UpdateData) (
	*domain.UpdateServiceSpec, error) {

	return &domain.UpdateServiceSpec{IsAsync: false}, ErrUpdateNotSupported
}

func (s *S3Provider) LastOperation(ctx context.Context, lastOperationData provideriface.LastOperationData) (
	*domain.LastOperation, error) {
	return &domain.LastOperation{State: brokerapi.Succeeded, Description: "Last operation polling not required. All operations are synchronous."}, nil
}
