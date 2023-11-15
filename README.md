# PaaS S3 Broker

A broker for AWS S3 buckets conforming to the [Open Service Broker API specification](https://github.com/openservicebrokerapi/servicebroker/blob/v2.14/spec.md).

The implementation creates an S3 bucket for every service instance and bindings are implemented as an IAM user with access keys. Access to the buckets is granted via bucket policies which name a specific set of users: one for each binding.

## Requirements

The IAM role for the broker must include at least the following policy:

```json
{
    "Version": "2012-10-17",
    "Statement": [{
            "Action": [
                "s3:CreateBucket",
                "s3:DeleteBucket",
                "s3:PutBucketPolicy",
                "s3:PutBucketPublicAccessBlock",
                "s3:DeleteBucketPolicy",
                "s3:GetBucketPolicy",
                "s3:PutBucketTagging",
                "s3:PutEncryptionConfiguration",
                "s3:GetEncryptionConfiguration"
            ],
            "Effect": "Allow",
            "Resource": "arn:aws:s3:::paas-s3-broker-*"
        },
        {
            "Action": [
                "iam:CreateUser",
                "iam:DeleteUser",
                "iam:*AccessKey*",
                "iam:TagUser",
                "iam:AttachUserPolicy",
                "iam:DetachUserPolicy",
                "iam:ListAttachedUserPolicies"
            ],
            "Effect": "Allow",
            "Resource": [
                "arn:aws:iam::*:user/paas-s3-broker/*"
            ]
        }
    ]
}
```

A policy must exist with at least these permissions (for IP restriction):

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Deny",
      "Resource": "*",
      "Action": "s3:*",
      "Condition": {
        "NotIpAddress": {
          "aws:SourceIp": [
          "ip.of.nat.gateway1/32",
          "ip.of.nat.gateway2/32",
          ...
          ]
        }
      }
    }
  ]
}

```

### Security

Given the policy above, the broker will not have the ability to create IAM policies - only bucket policies.

Unwanted access to S3 or IAM resources will be protected by using a couple of namespaces:

1. The S3 `Resource` which can be managed by the broker will be limited to `arn:aws:s3:::bucket-name-prefix-*`. This stops it being able to affect other buckets that may be in the same account.
2. The users the broker will be able to manage will be limited to `arn:aws:iam::*:user/s3-broker/*`. This namespace is hardcoded.

Also, by the nature of bucket policies, a full user name and bucket name have to be provided. This means unnecessarily broad access permissions cannot be granted.

Here is an example bucket policy the broker will apply:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Action": [
				"s3:GetBucketLocation",
				"s3:ListBucket",
				"s3:GetObject",
				"s3:PutObject",
				"s3:DeleteObject"
      ],
      "Effect": "Allow",
      "Resource": [
      	"arn:aws:s3:::paas-s3-broker-instance-id",
      	"arn:aws:s3:::paas-s3-broker-instance-id/*"
      ],
      "Principal": {
        "AWS": "arn:aws:iam::<account-number>:user/paas-s3-broker/some-user-id"
      }
    }
  ]
}
```

An additional policy can be supplied in the `iam_common_user_policy_arn`
configuration option and this policy will be applied to all users the broker
creates. Great care should be taken that this policy doesn't inadvertantly
grant more privileges than desired. The intended use of this feature was
to allow the IAM users to copy between buckets in other AWS accounts using
a policy such as:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": "s3:*",
            "Resource": "*",
            "Condition": {
                "StringNotEquals": {
                    "aws:ResourceAccount": "<platform-account-id>"
                }
            }
        }
    ]
}
```

Notably excluding the platform's own AWS account to prevent it from granting
it access to any other resources in the platform's account.

A policy ARN can be supplied in the `iam_user_permissions_boundary_arn`
option which will be applied as a permissions boundary for created binding
users. This can be used as an extra level of assurance that created users
abilities will be limited.

## Running

Minimal example:

```bash
go run main.go -config examples/config.json
```

### Configuration options

The following options can be added to the configuration file:

| Field                               | Default value | Type   | Values                                                                     |
| ----------------------------------- | ------------- | ------ | -------------------------------------------------------------------------- |
| `basic_auth_username`               | empty string  | string | any non-empty string                                                       |
| `basic_auth_password`               | empty string  | string | any non-empty string                                                       |
| `port`                              | 3000          | string | any free port                                                              |
| `log_level`                         | debug         | string | debug,info,error,fatal                                                     |
| `aws_region`                        | empty string  | string | any [AWS region](https://docs.aws.amazon.com/general/latest/gr/rande.html) |
| `bucket_prefix`                     | empty string  | string | any                                                                        |
| `iam_user_path`                     | empty string  | string | it should be in "/path/" format                                            |
| `iam_ip_restriction_policy_arn`     | empty string  | string | an AWS ARN of the IP restriction policy                                    |
| `iam_common_user_policy_arn`        | empty string  | string | an AWS ARN of an IAM policy to attach to all created users                 |
| `iam_user_permissions_boundary_arn` | empty string  | string | an AWS ARN of an IAM policy apply as created users' permissions boundary   |

## Testing

Run unit tests with:

```make
make unit
```

Run all tests, including integration tests, with:

```make
make test
```

The integration tests will require you to have at least the IAM permissions listed in the above [requirements](#requirements) section.

## `costs_by_month` utility

In `cmd/costs_by_month/README.md` you can find instructions for calculating the cost of tenant S3 buckets over the last few months.

## Patching an existing bosh environment

If you want to patch an existing bosh environment you can run the following command:

```
make bosh_scp
```

This requires an existing bosh session to be established beforehand.
