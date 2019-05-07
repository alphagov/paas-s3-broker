package broker_test

import (
	"code.cloudfoundry.org/lager"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	aws_s3 "github.com/aws/aws-sdk-go/service/s3"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"encoding/json"
	"net/http"
	"os"

	"net/http/httptest"

	"github.com/alphagov/paas-s3-broker/provider"
	"github.com/alphagov/paas-s3-broker/s3"
	"github.com/alphagov/paas-s3-broker/testing/integration/helpers"
	"github.com/alphagov/paas-service-broker-base/broker"
	brokertesting "github.com/alphagov/paas-service-broker-base/testing"
	"github.com/pivotal-cf/brokerapi"
	uuid "github.com/satori/go.uuid"
)

const (
	ASYNC_ALLOWED = true
)

type BindingResponse struct {
	Credentials map[string]interface{} `json:"credentials"`
}

var _ = Describe("Broker", func() {

	var (
		instanceID string
		binding1ID string
		binding2ID string
		serviceID  = "uuid-1"
		planID     = "uuid-2"
	)

	BeforeEach(func() {
		instanceID = uuid.NewV4().String()
		binding1ID = uuid.NewV4().String()
		binding2ID = uuid.NewV4().String()
	})

	It("should return a 410 response when trying to delete a non-existent instance", func() {
		_, brokerTester := initialise(*BrokerSuiteData.LocalhostIAMPolicyArn)

		res := brokerTester.Deprovision(instanceID, serviceID, planID, ASYNC_ALLOWED)
		Expect(res.Code).To(Equal(http.StatusGone))
	})

	It("should manage the lifecycle of an S3 bucket", func() {
		By("initialising")
		s3ClientConfig, brokerTester := initialise(*BrokerSuiteData.LocalhostIAMPolicyArn)

		By("Provisioning")
		res := brokerTester.Provision(instanceID, brokertesting.RequestBody{
			ServiceID: serviceID,
			PlanID:    planID,
		}, ASYNC_ALLOWED)
		Expect(res.Code).To(Equal(http.StatusCreated))

		defer helpers.DeprovisionService(brokerTester, instanceID, serviceID, planID)

		By("Binding an app")
		res = brokerTester.Bind(instanceID, binding1ID, brokertesting.RequestBody{
			ServiceID: serviceID,
			PlanID:    planID,
			Parameters: &brokertesting.ConfigurationValues{
				// We must allow external access with these credentials, because the tests do not run from a diego cell
				"allow_external_access": true,
			},
		}, ASYNC_ALLOWED)
		Expect(res.Code).To(Equal(http.StatusCreated))

		defer helpers.Unbind(brokerTester, instanceID, serviceID, planID, binding1ID)

		By("Asserting the credentials returned work for both reading and writing")
		readWriteBindingCreds := extractCredentials(res)
		helpers.AssertBucketReadWriteAccess(readWriteBindingCreds, s3ClientConfig.ResourcePrefix, instanceID, s3ClientConfig.AWSRegion)

		By("Binding an app as a read-only user")
		helpers.WriteTempFile(readWriteBindingCreds, s3ClientConfig.ResourcePrefix, instanceID, s3ClientConfig.AWSRegion)
		res = brokerTester.Bind(instanceID, binding2ID, brokertesting.RequestBody{
			ServiceID: serviceID,
			PlanID:    planID,
			Parameters: &brokertesting.ConfigurationValues{
				"permissions": "read-only",
				// We must allow external access with these credentials, because the tests do not run from a diego cell
				"allow_external_access": true,
			},
		}, ASYNC_ALLOWED)
		Expect(res.Code).To(Equal(http.StatusCreated))

		defer helpers.Unbind(brokerTester, instanceID, serviceID, planID, binding2ID)

		By("Asserting that read-only credentials can read, but fail to write to a file")
		readOnlyBindingCreds := extractCredentials(res)
		helpers.AssertBucketReadOnlyAccess(readOnlyBindingCreds, s3ClientConfig.ResourcePrefix, instanceID, s3ClientConfig.AWSRegion)

		By("Asserting the first user's credentials still work for reading and writing")
		helpers.AssertBucketReadWriteAccess(readWriteBindingCreds, s3ClientConfig.ResourcePrefix, instanceID, s3ClientConfig.AWSRegion)
	})

	It("manages public buckets correctly", func() {
		By("initialising")
		s3ClientConfig, brokerTester := initialise(*BrokerSuiteData.LocalhostIAMPolicyArn)

		By("provisioning a public bucket")
		res := brokerTester.Provision(instanceID, brokertesting.RequestBody{
			ServiceID:  serviceID,
			PlanID:     planID,
			Parameters: &brokertesting.ConfigurationValues{"public_bucket": true},
		}, ASYNC_ALLOWED)
		Expect(res.Code).To(Equal(http.StatusCreated))

		defer helpers.DeprovisionService(brokerTester, instanceID, serviceID, planID)

		By("binding an app as a read-write user")
		res = brokerTester.Bind(instanceID, binding2ID, brokertesting.RequestBody{
			ServiceID: serviceID,
			PlanID:    planID,
			Parameters: &brokertesting.ConfigurationValues{
				"permissions": "read-write",
				// We must allow external access with these credentials, because the tests do not run from a diego cell
				"allow_external_access": true,
			},
		}, ASYNC_ALLOWED)
		Expect(res.Code).To(Equal(http.StatusCreated))
		defer helpers.Unbind(brokerTester, instanceID, serviceID, planID, binding2ID)

		By("asserting the credentials returned work for both reading and writing")
		readWriteBindingCreds := extractCredentials(res)
		helpers.AssertBucketReadWriteAccess(readWriteBindingCreds, s3ClientConfig.ResourcePrefix, instanceID, s3ClientConfig.AWSRegion)

		By("writing temp file to the bucket")
		helpers.WriteTempFile(readWriteBindingCreds, s3ClientConfig.ResourcePrefix, instanceID, s3ClientConfig.AWSRegion)
		defer func() {
			helpers.DeleteTempFile(readWriteBindingCreds, s3ClientConfig.ResourcePrefix, instanceID, s3ClientConfig.AWSRegion)
		}()

		By("asserting we can GET the file over unauthenticated HTTP")
		resp, err := http.Get(fmt.Sprintf("http://%s.s3.amazonaws.com/%s", s3ClientConfig.ResourcePrefix+instanceID, helpers.TestFileKey))
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})

	It("manages private buckets correctly", func() {
		By("initialising")
		s3ClientConfig, brokerTester := initialise(*BrokerSuiteData.LocalhostIAMPolicyArn)

		By("provisioning a private bucket")
		res := brokerTester.Provision(instanceID, brokertesting.RequestBody{
			ServiceID: serviceID,
			PlanID:    planID,
			Parameters: &brokertesting.ConfigurationValues{
				"public_bucket": false,
			},
		}, ASYNC_ALLOWED)
		Expect(res.Code).To(Equal(http.StatusCreated))

		defer helpers.DeprovisionService(brokerTester, instanceID, serviceID, planID)

		By("binding an app as a read-write user")
		res = brokerTester.Bind(instanceID, binding2ID, brokertesting.RequestBody{
			ServiceID: serviceID,
			PlanID:    planID,
			Parameters: &brokertesting.ConfigurationValues{
				"permissions": "read-write",
				// We must allow external access with these credentials, because the tests do not run from a diego cell
				"allow_external_access": true,
			},
		}, ASYNC_ALLOWED)
		Expect(res.Code).To(Equal(http.StatusCreated))
		defer helpers.Unbind(brokerTester, instanceID, serviceID, planID, binding2ID)

		By("asserting the credentials returned work for both reading and writing")
		readWriteBindingCreds := extractCredentials(res)
		helpers.AssertBucketReadWriteAccess(readWriteBindingCreds, s3ClientConfig.ResourcePrefix, instanceID, s3ClientConfig.AWSRegion)

		By("writing temp file to the bucket")
		helpers.WriteTempFile(readWriteBindingCreds, s3ClientConfig.ResourcePrefix, instanceID, s3ClientConfig.AWSRegion)
		defer func() {
			helpers.DeleteTempFile(readWriteBindingCreds, s3ClientConfig.ResourcePrefix, instanceID, s3ClientConfig.AWSRegion)
		}()

		By("asserting we cannot GET the file over unauthenticated HTTP")
		resp, err := http.Get(fmt.Sprintf("http://%s.s3.amazonaws.com/%s", s3ClientConfig.ResourcePrefix+instanceID, helpers.TestFileKey))
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
	})

	Context("With an IAM policy that does not include the IP the test is running from", func() {
		It("should create credentials that cannot be used", func() { //these integration tests are run from concourse, which do not use the NAT gateways
			By("initialising")
			s3ClientConfig, brokerTester := initialise(*BrokerSuiteData.LocalhostIAMPolicyArn)

			By("provisioning a private bucket")
			res := brokerTester.Provision(instanceID, brokertesting.RequestBody{
				ServiceID: serviceID,
				PlanID:    planID,
				Parameters: &brokertesting.ConfigurationValues{
					"public_bucket": false,
				},
			}, ASYNC_ALLOWED)
			Expect(res.Code).To(Equal(http.StatusCreated))

			defer helpers.DeprovisionService(brokerTester, instanceID, serviceID, planID)

			By("binding an app as a read-write user with external access enabled")
			res = brokerTester.Bind(instanceID, binding1ID, brokertesting.RequestBody{
				ServiceID: serviceID,
				PlanID:    planID,
				Parameters: &brokertesting.ConfigurationValues{
					"permissions": "read-write",
					// We must allow external access with these credentials, because the tests do not run from a diego cell
					"allow_external_access": true,
				},
			}, ASYNC_ALLOWED)
			Expect(res.Code).To(Equal(http.StatusCreated))
			binding1Creds := extractCredentials(res)

			defer helpers.Unbind(brokerTester, instanceID, serviceID, planID, binding1ID)

			By("asserting the credentials returned work remotely")
			helpers.AssertBucketReadWriteAccess(binding1Creds, s3ClientConfig.ResourcePrefix, instanceID, s3ClientConfig.AWSRegion)

			By("binding an app as a read-write user without external access enabled")
			res = brokerTester.Bind(instanceID, binding2ID, brokertesting.RequestBody{
				ServiceID: serviceID,
				PlanID:    planID,
				Parameters: &brokertesting.ConfigurationValues{
					"permissions": "read-write",
					// We must allow external access with these credentials, because the tests do not run from a diego cell
					"allow_external_access": false,
				},
			}, ASYNC_ALLOWED)
			Expect(res.Code).To(Equal(http.StatusCreated))
			binding2Creds := extractCredentials(res)

			defer helpers.Unbind(brokerTester, instanceID, serviceID, planID, binding2ID)

			helpers.AssertNoBucketAccess(binding2Creds, s3ClientConfig.ResourcePrefix, instanceID, s3ClientConfig.AWSRegion)
		})
	})

	Context("With an IAM policy that includes the IP the test is running from", func() {
		It("should create credentials that can be used", func() { //these integration tests are run from concourse, which do not use the NAT gateways
			By("initialising")
			s3ClientConfig, brokerTester := initialise(*BrokerSuiteData.EgressIPIAMPolicyARN)

			By("provisioning a private bucket")
			res := brokerTester.Provision(instanceID, brokertesting.RequestBody{
				ServiceID: serviceID,
				PlanID:    planID,
				Parameters: &brokertesting.ConfigurationValues{
					"public_bucket": false,
				},
			}, ASYNC_ALLOWED)
			Expect(res.Code).To(Equal(http.StatusCreated))

			defer helpers.DeprovisionService(brokerTester, instanceID, serviceID, planID)

			By("binding an app as a read-write user with external access enabled")
			res = brokerTester.Bind(instanceID, binding1ID, brokertesting.RequestBody{
				ServiceID: serviceID,
				PlanID:    planID,
				Parameters: &brokertesting.ConfigurationValues{
					"permissions": "read-write",
					// We must allow external access with these credentials, because the tests do not run from a diego cell
					"allow_external_access": true,
				},
			}, ASYNC_ALLOWED)
			Expect(res.Code).To(Equal(http.StatusCreated))
			binding1Creds := extractCredentials(res)

			defer helpers.Unbind(brokerTester, instanceID, serviceID, planID, binding1ID)

			By("asserting the credentials returned work remotely")
			helpers.AssertBucketReadWriteAccess(binding1Creds, s3ClientConfig.ResourcePrefix, instanceID, s3ClientConfig.AWSRegion)

			By("binding an app as a read-write user without external access enabled")
			res = brokerTester.Bind(instanceID, binding2ID, brokertesting.RequestBody{
				ServiceID: serviceID,
				PlanID:    planID,
				Parameters: &brokertesting.ConfigurationValues{
					"permissions": "read-write",
					// We must allow external access with these credentials, because the tests do not run from a diego cell
					"allow_external_access": false,
				},
			}, ASYNC_ALLOWED)
			Expect(res.Code).To(Equal(http.StatusCreated))
			binding2Creds := extractCredentials(res)

			defer helpers.Unbind(brokerTester, instanceID, serviceID, planID, binding2ID)
			helpers.AssertBucketReadWriteAccess(binding2Creds, s3ClientConfig.ResourcePrefix, instanceID, s3ClientConfig.AWSRegion)
		})
	})
})

func initialise(IAMPolicyARN string) (*s3.Config, brokertesting.BrokerTester) {
	file, err := os.Open("../../fixtures/config.json")
	Expect(err).ToNot(HaveOccurred())
	defer file.Close()

	config, err := broker.NewConfig(file)
	Expect(err).ToNot(HaveOccurred())

	s3ClientConfig, err := s3.NewS3ClientConfig(config.Provider)
	Expect(err).ToNot(HaveOccurred())

	s3ClientConfig.IpRestrictionPolicyARN = IAMPolicyARN
	Expect(s3ClientConfig.IpRestrictionPolicyARN).To(HavePrefix("arn:aws:iam::"))

	logger := lager.NewLogger("s3-service-broker-test")
	logger.RegisterSink(lager.NewWriterSink(GinkgoWriter, config.API.LagerLogLevel))

	sess := session.Must(session.NewSession(&aws.Config{Region: aws.String(s3ClientConfig.AWSRegion)}))
	s3Client := s3.NewS3Client(s3ClientConfig, aws_s3.New(sess), iam.New(sess), logger)

	s3Provider := provider.NewS3Provider(s3Client)

	serviceBroker := broker.New(config, s3Provider, logger)
	brokerAPI := broker.NewAPI(serviceBroker, logger, config)

	return s3ClientConfig, brokertesting.New(brokerapi.BrokerCredentials{
		Username: "username",
		Password: "password",
	}, brokerAPI)
}

func extractCredentials(res *httptest.ResponseRecorder) s3.BucketCredentials {
	parsedResponse := BindingResponse{}
	err := json.NewDecoder(res.Body).Decode(&parsedResponse)
	Expect(err).ToNot(HaveOccurred())
	// Ensure returned credentials follow guidlines in https://docs.cloudfoundry.org/services/binding-credentials.html
	var str string
	creds := parsedResponse.Credentials
	Expect(creds).To(HaveKeyWithValue("bucket_name", BeAssignableToTypeOf(str)))
	Expect(creds).To(HaveKeyWithValue("aws_access_key_id", BeAssignableToTypeOf(str)))
	Expect(creds).To(HaveKeyWithValue("aws_secret_access_key", BeAssignableToTypeOf(str)))
	return s3.BucketCredentials{
		BucketName:         creds["bucket_name"].(string),
		AWSAccessKeyID:     creds["aws_access_key_id"].(string),
		AWSSecretAccessKey: creds["aws_secret_access_key"].(string),
	}
}
