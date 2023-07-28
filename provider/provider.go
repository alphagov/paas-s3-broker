package provider

import (
	"context"
	"errors"

	"github.com/alphagov/paas-s3-broker/s3"
	provideriface "github.com/alphagov/paas-service-broker-base/provider"
	"github.com/pivotal-cf/brokerapi/domain"
	"github.com/pivotal-cf/brokerapi/domain/apiresponses"
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
	res *domain.ProvisionedServiceSpec, err error) {

	err = s.client.CreateBucket(provisionData)
	res = &domain.ProvisionedServiceSpec{IsAsync: false, AlreadyExists: false, DashboardURL: "", OperationData: ""}
	return res, err
}

func (s *S3Provider) Deprovision(ctx context.Context, deprovisionData provideriface.DeprovisionData) (
	res *domain.DeprovisionServiceSpec, err error) {

	err = s.client.DeleteBucket(deprovisionData.InstanceID)
	res = &domain.DeprovisionServiceSpec{IsAsync: false, OperationData: ""}
	if err == s3.ErrNoSuchResources {
		return res, apiresponses.ErrInstanceDoesNotExist
	}
	return res, err
}

func (s *S3Provider) Bind(ctx context.Context, bindData provideriface.BindData) (
	binding *domain.Binding, err error) {

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
	unbinding *domain.UnbindSpec, err error) {

	err = s.client.RemoveUserFromBucketAndDeleteUser(unbindData.BindingID, unbindData.InstanceID)
	if err != nil {
		if err == s3.ErrNoSuchResources {
			return &domain.UnbindSpec{}, apiresponses.ErrBindingDoesNotExist
		}
		return &domain.UnbindSpec{}, err
	}
	return &domain.UnbindSpec{
		IsAsync: false,
	}, nil
}

var ErrUpdateNotSupported = errors.New("Updating the S3 bucket is currently not supported")

func (s *S3Provider) Update(ctx context.Context, updateData provideriface.UpdateData) (
	res *domain.UpdateServiceSpec, err error) {
		return &domain.UpdateServiceSpec{IsAsync: false,DashboardURL: "", OperationData: ""}, ErrUpdateNotSupported
}

func (s *S3Provider) LastOperation(ctx context.Context, lastOperationData provideriface.LastOperationData) (
	state *domain.LastOperation, err error) {
	return &domain.LastOperation{State: domain.Succeeded, Description: "Last operation polling not required. All operations are synchronous."}, nil
}
