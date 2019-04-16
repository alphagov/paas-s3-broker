package policy_test

import (
	"github.com/alphagov/paas-s3-broker/s3/policy"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("PolicyBuilder", func() {
	Context("when the existing policy is not empty", func() {
		It("should return an error if passed incorrect JSON", func() {
			policyDocument, err := policy.BuildPolicy(
				`{"some-json": "that-does-not-match-the-format"}`,
				policy.Statement{},
			)
			Expect(err).To(HaveOccurred())
			Expect(policyDocument).To(Equal(policy.PolicyDocument{}))
		})

		It("should append the provided statement to a policy with other statements", func() {
			statement := policy.Statement{}
			policyDocument, err := policy.BuildPolicy(
				`{
					"Version":"2012-10-17",
					"Statement":[
						{
							"Effect": "Allow",
							"Principal": {
								"AWS": "arn:aws:sts::some-principal"
							},
							"Action": [
								"s3:ListBucket",
								"s3:GetObject"
							],
							"Resource": [
								"arn:aws:s3:::some-instance-id",
								"arn:aws:s3:::some-instance-id/*"
							]
						}
					]
				}`,
				statement,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(policyDocument.Statement).To(HaveLen(2))
			Expect(policyDocument.Statement[0].Principal.AWS).To(Equal("arn:aws:sts::some-principal"))
			Expect(policyDocument.Statement[1]).To(Equal(statement))
		})

		It("should append the provided statement to an empty policy", func() {
			statement := policy.Statement{}
			policyDocument, err := policy.BuildPolicy(
				`{"Version":"2012-10-17","Statement":[]}`,
				statement,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(policyDocument.Statement).To(ConsistOf(statement))
		})
	})

	Context("when the existing policy is empty", func() {

		It("should return a new policy with the provided statement", func() {
			statement := policy.Statement{}
			policyDocument, err := policy.BuildPolicy(
				"",
				statement,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(policyDocument.Statement).To(ConsistOf(statement))
		})

	})

	Context("removing statements from a policy", func() {
		Context("when an existing policy is not empty", func() {
			It("should return an error if passed incorrect JSON", func() {
				_, err := policy.RemoveUserFromPolicy(`{"crap": "json"}`, "some-arn")
				Expect(err).To(HaveOccurred())
			})

			It("when an existing policy has no statements, returns an error", func() {
				_, err := policy.RemoveUserFromPolicy(
					`{"Version": "2012-10-17", "Statement":[]}`,
					"some-arn",
				)
				Expect(err).To(HaveOccurred())
			})

			It("when an existing policy does not have a statement relating to the given user, returns an error", func() {
				_, err := policy.RemoveUserFromPolicy(`{
					"Version":"2012-10-17",
					"Statement":[
						{
							"Effect": "Allow",
							"Principal": {
								"AWS": "arn:aws:sts::some-principal"
							},
							"Action": [
								"s3:ListBucket",
								"s3:GetObject"
							],
							"Resource": [
								"arn:aws:s3:::some-instance-id",
								"arn:aws:s3:::some-instance-id/*"
							]
						}
					]
				}`, "some-arn")

				Expect(err).To(HaveOccurred())
			})

			It("when an existing policy has a statement relating to the given user, returns the document without the statement", func() {
				document, err := policy.RemoveUserFromPolicy(`{
					"Version":"2012-10-17",
					"Statement":[
						{
							"Effect": "Allow",
							"Principal": {
								"AWS": "arn:aws:sts::some-arn"
							},
							"Action": [
								"s3:ListBucket",
								"s3:GetObject"
							],
							"Resource": [
								"arn:aws:s3:::some-instance-id",
								"arn:aws:s3:::some-instance-id/*"
							]
						},
						{
							"Effect": "Allow",
							"Principal": {
								"AWS": "arn:aws:sts::some-other-arn"
							},
							"Action": [
								"s3:ListBucket",
								"s3:GetObject"
							],
							"Resource": [
								"arn:aws:s3:::some-instance-id",
								"arn:aws:s3:::some-instance-id/*"
							]
						}
					]
				}`, "arn:aws:sts::some-arn")

				Expect(err).ToNot(HaveOccurred())
				Expect(document.Statement).To(HaveLen(1))
				Expect(document.Statement[0].Principal.AWS).To(Equal("arn:aws:sts::some-other-arn"))
			})
		})

		Context("when an existing policy is empty", func() {
			It("returns an error. The policy should not be empty when removing a user.", func() {
				_, err := policy.RemoveUserFromPolicy("", "some-arn")
				Expect(err).To(HaveOccurred())
			})
		})

	})
})
