# PaaS S3 Broker

A broker for AWS S3 buckets conforming to the [Open Service Broker API specification](https://github.com/openservicebrokerapi/servicebroker/blob/v2.14/spec.md).

The implementation creates an S3 bucket for every service instance and bindings are implemented as an IAM user with access keys. Access to the buckets is granted via bucket policies which name a specific set of users: one for each binding.

## Requirements

The IAM role for the broker must include at least the following policy:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Action": [
        "s3:CreateBucket",
        "s3:DeleteBucket",
        "s3:PutBucketPolicy",
        "s3:DeleteBucketPolicy",
        "s3:GetBucketPolicy"
      ],
      "Effect": "Allow",
      "Resource": "arn:aws:s3:::bucket-name-prefix-*"
    },
    {
      "Action": [
        "iam:CreateUser",
        "iam:DeleteUser",
        "iam:*AccessKey*"
      ],
      "Effect": "Allow",
      "Resource": [
        "arn:aws:iam::*:user/s3-broker/*"
      ]
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
        "s3:DeleteObject",
        "s3:GetObject",
        "s3:PutObject"
      ],
      "Effect": "Allow",
      "Resource": "arn:aws:s3:::bucket-name-prefix-instance-id/*",
      "Principal": {
        "AWS": "arn:aws:iam::<account-number>:user/s3-broker/some-user-id"
      }
    }
  ]
}
```

## Running

Minimal example:

```bash
go run main.go -config examples/config.json
```

### Configuration options

The following options can be added to the configuration file:

| Field | Default value | Type | Values |
|---|---|---|---|
| `basic_auth_username` | empty string | string | any non-empty string |
| `basic_auth_password` | empty string | string | any non-empty string |
| `port` | 3000 | string | any free port |
| `log_level` | debug | string | debug,info,error,fatal |
| `aws_region` | empty string | string | any [AWS region](https://docs.aws.amazon.com/general/latest/gr/rande.html) |
| `bucket_prefix` | empty string | string | any |

## Testing

Run unit tests with:

```make
make unit
```

Run all tests, including integration tests, with:

```make
make test
```

The integration tests will require you to have at least the IAM policy listed in the above [requirements](#requirements) section.