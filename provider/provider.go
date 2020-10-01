package provider

import (
	"context"
	"errors"
	"strings"

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
	dashboardURL, operationData string, isAsync bool, err error) {

	err = s.client.CreateBucket(provisionData)

	return "", "", false, err
}

func (s *S3Provider) Deprovision(ctx context.Context, deprovisionData provideriface.DeprovisionData) (
	operationData string, isAsync bool, err error) {

	err = s.client.DeleteBucket(deprovisionData.InstanceID)
	if err != nil {
		if strings.Contains(err.Error(), "NoSuchBucket: The specified bucket does not exist") {
			return "", false, brokerapi.ErrInstanceDoesNotExist
		}
	}
	return "", false, err
}

func (s *S3Provider) Bind(ctx context.Context, bindData provideriface.BindData) (
	binding domain.Binding, err error) {

	bucketCredentials, volumeMounts, err := s.client.AddUserToBucket(bindData)
	if err != nil {
		return domain.Binding{}, err
	}

	return domain.Binding{
		IsAsync:      false,
		Credentials:  bucketCredentials,
		VolumeMounts: volumeMounts,
	}, nil
}

func (s *S3Provider) Unbind(ctx context.Context, unbindData provideriface.UnbindData) (
	unbinding domain.UnbindSpec, err error) {

	err = s.client.RemoveUserFromBucketAndDeleteUser(unbindData.BindingID, unbindData.InstanceID)
	if err != nil {
		return domain.UnbindSpec{}, err
	}
	return domain.UnbindSpec{
		IsAsync: false,
	}, nil
}

var ErrUpdateNotSupported = errors.New("Updating the S3 bucket is currently not supported")

func (s *S3Provider) Update(ctx context.Context, updateData provideriface.UpdateData) (
	operationData string, isAsync bool, err error) {
	return "", false, ErrUpdateNotSupported
}

func (s *S3Provider) LastOperation(ctx context.Context, lastOperationData provideriface.LastOperationData) (
	state domain.LastOperationState, description string, err error) {
	return domain.Succeeded, "Last operation polling not required. All operations are synchronous.", nil
}
