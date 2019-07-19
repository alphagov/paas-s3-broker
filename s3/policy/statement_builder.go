package policy

import (
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
)

type Statement struct {
	Effect    string    `json:"Effect"`
	Action    Actions  `json:"Action"`
	Resource  []string  `json:"Resource"`
	Principal Principal `json:"Principal"`
}

// We alias Actions as []string here and
// implement the UnmarshalJSON interface
// because AWS IAM policies with a single
// action are returned with a string,
// instead of an array with a single element,
// and Go's type system is no expressive enough
// to support that.
type Actions []string
func (this *Actions) UnmarshalJSON(b []byte) error {
	var actions []string
	err := json.Unmarshal(b, &actions)
	if err == nil {
		*this = actions
		return nil
	}
	var singleAction string
	newerr := json.Unmarshal(b, &singleAction)
	if newerr != nil {
		return newerr
	}
	*this = Actions{singleAction}
	return nil
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
		"s3:GetObjectAcl",
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
		"s3:GetObjectAcl",
		"s3:PutObject",
		"s3:PutObjectAcl",
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
