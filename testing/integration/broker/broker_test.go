package broker_test

import (
	"code.cloudfoundry.org/lager"
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

	"github.com/alphagov/paas-go/broker"
	brokertesting "github.com/alphagov/paas-go/testing/broker"
	"github.com/alphagov/paas-s3-broker/provider"
	"github.com/alphagov/paas-s3-broker/s3"
	"github.com/alphagov/paas-s3-broker/testing/integration/helpers"
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

	It("should manage the lifecycle of an S3 bucket", func() {
		By("initialising")
		s3ClientConfig, brokerTester := initialise()

		By("Provisioning")
		res := brokerTester.Provision(instanceID, brokertesting.RequestBody{
			ServiceID: serviceID,
			PlanID:    planID,
		}, ASYNC_ALLOWED)
		Expect(res.Code).To(Equal(http.StatusCreated))

		defer func() {
			By("Deprovisioning")
			res = brokerTester.Deprovision(instanceID, serviceID, planID, ASYNC_ALLOWED)
			Expect(res.Code).To(Equal(http.StatusOK))
		}()

		By("Binding an app")
		res = brokerTester.Bind(instanceID, binding1ID, brokertesting.RequestBody{
			ServiceID: serviceID,
			PlanID:    planID,
		}, ASYNC_ALLOWED)
		Expect(res.Code).To(Equal(http.StatusCreated))

		defer func() {
			By("Unbinding the first app")
			res = brokerTester.Unbind(instanceID, serviceID, planID, binding1ID, ASYNC_ALLOWED)
			Expect(res.Code).To(Equal(http.StatusOK))
		}()

		By("Asserting the credentials returned work")
		binding1Creds := extractCredentials(res)
		helpers.AssertBucketAccess(binding1Creds, s3ClientConfig.ResourcePrefix, instanceID, s3ClientConfig.AWSRegion)

		By("Binding another app")
		res = brokerTester.Bind(instanceID, binding2ID, brokertesting.RequestBody{
			ServiceID: serviceID,
			PlanID:    planID,
		}, ASYNC_ALLOWED)
		Expect(res.Code).To(Equal(http.StatusCreated))

		defer func() {
			By("Unbinding the second app")
			res = brokerTester.Unbind(instanceID, serviceID, planID, binding2ID, ASYNC_ALLOWED)
			Expect(res.Code).To(Equal(http.StatusOK))
		}()

		By("Asserting the credentials returned work")
		binding2Creds := extractCredentials(res)
		helpers.AssertBucketAccess(binding2Creds, s3ClientConfig.ResourcePrefix, instanceID, s3ClientConfig.AWSRegion)

		By("Asserting the first user's credentials still work")
		helpers.AssertBucketAccess(binding1Creds, s3ClientConfig.ResourcePrefix, instanceID, s3ClientConfig.AWSRegion)

		By("Asserting the second user's credentials still work")
		helpers.AssertBucketAccess(binding2Creds, s3ClientConfig.ResourcePrefix, instanceID, s3ClientConfig.AWSRegion)
	})

	It("should return a 410 response when trying to delete a non-existent instanc", func() {
		_, brokerTester := initialise()

		res := brokerTester.Deprovision(instanceID, serviceID, planID, ASYNC_ALLOWED)
		Expect(res.Code).To(Equal(http.StatusGone))
	})
})

func initialise() (*s3.Config, brokertesting.BrokerTester) {
	file, err := os.Open("../../fixtures/config.json")
	Expect(err).ToNot(HaveOccurred())
	defer file.Close()

	config, err := broker.NewConfig(file)
	Expect(err).ToNot(HaveOccurred())

	s3ClientConfig, err := s3.NewS3ClientConfig(config.Provider)
	Expect(err).ToNot(HaveOccurred())

	logger := lager.NewLogger("s3-service-broker-test")
	logger.RegisterSink(lager.NewWriterSink(os.Stdout, config.API.LagerLogLevel))

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
