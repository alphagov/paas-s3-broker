package policy_test

import (
	"github.com/alphagov/paas-s3-broker/s3/policy"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("StatementBuilder", func() {
	It("should build a statement that gives read only permissions", func() {
		actualStatement := policy.BuildStatement(
			"some-instance-id",
			iam.User{Arn: aws.String("some-arn")},
			policy.ReadOnlyPermissions{})

		Expect(actualStatement.Effect).To(Equal("Allow"))
		Expect(actualStatement.Principal.AWS).To(Equal("some-arn"))
		Expect(actualStatement.Resource).To(HaveLen(2))
		Expect(actualStatement.Resource).To(ContainElement("arn:aws:s3:::some-instance-id"))
		Expect(actualStatement.Resource).To(ContainElement("arn:aws:s3:::some-instance-id/*"))
		Expect(actualStatement.Action).To(ConsistOf(
			"s3:GetObject",
			"s3:GetBucketLocation",
			"s3:ListBucket"),
		)
	})

	It("should build a statement that gives read and write permissions", func() {
		actualStatement := policy.BuildStatement(
			"some-instance-id",
			iam.User{Arn: aws.String("some-arn")},
			policy.ReadWritePermissions{})

		Expect(actualStatement.Effect).To(Equal("Allow"))
		Expect(actualStatement.Principal.AWS).To(Equal("some-arn"))
		Expect(actualStatement.Resource).To(HaveLen(2))
		Expect(actualStatement.Resource).To(ContainElement("arn:aws:s3:::some-instance-id"))
		Expect(actualStatement.Resource).To(ContainElement("arn:aws:s3:::some-instance-id/*"))
		Expect(actualStatement.Action).To(ConsistOf(
			"s3:GetBucketLocation",
			"s3:ListBucket",
			"s3:GetObject",
			"s3:PutObject",
			"s3:DeleteObject",
		),
		)
	})
})
