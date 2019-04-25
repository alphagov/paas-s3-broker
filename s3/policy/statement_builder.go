package policy

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
)

type Statement struct {
	Effect    string    `json:"Effect"`
	Action    []string  `json:"Action"`
	Resource  []string  `json:"Resource"`
	Principal Principal `json:"Principal"`
}

type Principal struct {
	AWS string `json:"AWS"`
}

const (
	ReadOnlyPermissionsName  = "read-only"
	ReadWritePermissionsName = "read-write"
)

type Permissions interface {
	Actions() []string
}

type NoPermissions struct{}
type ReadOnlyPermissions struct{}
type PublicBucketPermissions struct{}
type ReadWritePermissions struct{}

func (NoPermissions) Actions() []string {
	return []string{}
}

func (ReadOnlyPermissions) Actions() []string {
	return []string{
		"s3:GetBucketLocation",
		"s3:ListBucket",
		"s3:GetObject",
	}
}

func (PublicBucketPermissions) Actions() []string {
	return []string{
		"s3:GetObject",
	}
}

func (ReadWritePermissions) Actions() []string {
	return []string{
		"s3:GetBucketLocation",
		"s3:ListBucket",
		"s3:GetObject",
		"s3:PutObject",
		"s3:DeleteObject",
	}
}

func ValidatePermissions(permissionName string) (Permissions, error) {
	if permissionName == ReadOnlyPermissionsName {
		return ReadOnlyPermissions{}, nil
	} else if permissionName == ReadWritePermissionsName {
		return ReadWritePermissions{}, nil
	} else {
		return NoPermissions{}, fmt.Errorf("unknown permission name %s", permissionName)
	}
}

func BuildStatement(bucketName string, iamUser iam.User, permissions Permissions) Statement {
	return Statement{
		Effect:    "Allow",
		Principal: Principal{AWS: aws.StringValue(iamUser.Arn)},
		Resource: []string{
			fmt.Sprintf("arn:aws:s3:::%s", bucketName),
			fmt.Sprintf("arn:aws:s3:::%s/*", bucketName),
		},
		Action: permissions.Actions(),
	}
}
