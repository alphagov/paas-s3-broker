module github.com/alphagov/paas-s3-broker

require (
	code.cloudfoundry.org/lager v2.0.0+incompatible
	github.com/alphagov/paas-service-broker-base v0.6.0
	github.com/aws/aws-sdk-go v1.27.1
	github.com/cloudfoundry-community/go-cfclient v0.0.0-20190808214049-35bcce23fc5f
	github.com/olekukonko/tablewriter v0.0.4
	github.com/onsi/ginkgo v1.8.0
	github.com/onsi/gomega v1.5.0
	github.com/pivotal-cf/brokerapi v6.4.2+incompatible
	github.com/satori/go.uuid v1.2.0
)

go 1.13
