package provider

import (
	"github.com/pivotal-cf/brokerapi"
	provideriface "github.com/alphagov/paas-go/provider"
	"context"
	"errors"
)

type S3Provider struct {}

func NewS3Provider(config []byte) (provideriface.ServiceProvider, error) {
	return &S3Provider{}, nil
}

func (s *S3Provider) Provision(ctx context.Context, provisionData provideriface.ProvisionData) (
	dashboardURL, operationData string, err error) {
	return "", "", errors.New("not implemented")
}

func (s *S3Provider) Deprovision(ctx context.Context, deprovisionData provideriface.DeprovisionData) (
	operationData string, err error) {
	return "", errors.New("not implemented")
}

func (s *S3Provider) Bind(ctx context.Context, bindData provideriface.BindData) (
	binding brokerapi.Binding, err error) {
		return brokerapi.Binding{}, errors.New("not implemented")
}

func (s *S3Provider) Unbind(ctx context.Context, unbindData provideriface.UnbindData) (
	err error) {
		return errors.New("not implemented")
}

func (s *S3Provider) Update(ctx context.Context, updateData provideriface.UpdateData) (
	operationData string, err error) {
		return "", errors.New("not implemented")
}

func (s *S3Provider) LastOperation(ctx context.Context, lastOperationData provideriface.LastOperationData) (
	state brokerapi.LastOperationState, description string, err error) {
		return "", "", errors.New("not implemented")
}
