package policy

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
	"log"
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
	ReadOnlyPermissionName  = "read-only"
	ReadWritePermissionName = "read-write"
)

func BuildStatement(bucketName string, iamUser iam.User, permissionName string) Statement {
	var actions []string

	if permissionName == ReadOnlyPermissionName {
		actions = []string{
			"s3:GetBucketLocation",
			"s3:ListBucket",
			"s3:GetObject",
		}
	} else if permissionName == ReadWritePermissionName {
		actions = []string{
			"s3:GetBucketLocation",
			"s3:ListBucket",
			"s3:GetObject",
			"s3:PutObject",
			"s3:DeleteObject",
		}
	} else {
		log.Panicf("unknown permission name %s", permissionName)
	}

	return Statement{
		Effect:    "Allow",
		Principal: Principal{AWS: aws.StringValue(iamUser.Arn)},
		Resource: []string{
			fmt.Sprintf("arn:aws:s3:::%s", bucketName),
			fmt.Sprintf("arn:aws:s3:::%s/*", bucketName),
		},
		Action: actions,
	}
}
