package provider

import (
	"context"
	"encoding/json"
	"errors"
	provideriface "github.com/alphagov/paas-go/provider"
	"github.com/alphagov/paas-s3-broker/s3"
	"github.com/pivotal-cf/brokerapi"
)

type S3Provider struct {
	Client s3.Client
	Config *Config
}

func NewS3Provider(configJSON []byte) (*S3Provider, error) {
	config := &Config{
		AWSRegion: "eu-west-2",
	}
	err := json.Unmarshal(configJSON, &config)
	if err != nil {
		return nil, err
	}

	client := s3.NewS3Client(config.BucketPrefix, config.AWSRegion)

	return &S3Provider{
		Client: client,
		Config: config,
	}, nil
}

type Config struct {
	AWSRegion    string `json:"aws_region"`
	BucketPrefix string `json:"bucket_prefix"`
}

func (s *S3Provider) Provision(ctx context.Context, provisionData provideriface.ProvisionData) (
	dashboardURL, operationData string, isAsync bool, err error) {

	err = s.Client.CreateBucket(provisionData.InstanceID)
	return "", "", true, err
}

func (s *S3Provider) Deprovision(ctx context.Context, deprovisionData provideriface.DeprovisionData) (
	operationData string, isAsync bool, err error) {

	err = s.Client.DeleteBucket(deprovisionData.InstanceID)
	return "", true, err
}

func (s *S3Provider) Bind(ctx context.Context, bindData provideriface.BindData) (
	binding brokerapi.Binding, err error) {
	return brokerapi.Binding{}, errors.New("not implemented")
}

func (s *S3Provider) Unbind(ctx context.Context, unbindData provideriface.UnbindData) (
	unbinding brokerapi.UnbindSpec, err error) {
	return brokerapi.UnbindSpec{}, errors.New("not implemented")
}

var ErrUpdateNotSupported = errors.New("Updating the S3 bucket is currently not supported")

func (s *S3Provider) Update(ctx context.Context, updateData provideriface.UpdateData) (
	operationData string, isAsync bool, err error) {
	return "", true, ErrUpdateNotSupported
}

func (s *S3Provider) LastOperation(ctx context.Context, lastOperationData provideriface.LastOperationData) (
	state brokerapi.LastOperationState, description string, err error) {
	return "", "", errors.New("not implemented")
}
