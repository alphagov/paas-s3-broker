package main

import (
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/costexplorer"
	cfclient "github.com/cloudfoundry-community/go-cfclient"
	"github.com/olekukonko/tablewriter"
)

func main() {
	var cfApiUrl, cfApiToken string
	flag.StringVar(&cfApiUrl, "cf-api-url", "", "URL of Cloud Controller API")
	flag.StringVar(&cfApiToken, "cf-api-token", "", "OAuth2 Token for the Cloud Controller API")
	flag.Parse()

	cf, err := cfClient(cfApiUrl, cfApiToken)
	if err != nil {
		log.Fatalln(err)
	}
	serviceInstances, err := cfGetServiceInstancesByServiceLabel(cf, "aws-s3-bucket")
	if err != nil {
		log.Fatalln(err)
	}
	orgs, spaces, err := cfGetSpacesAndOrgsFromServiceInstances(cf, serviceInstances)
	if err != nil {
		log.Fatalln(err)
	}

	// Calculate start date of this month and the three previous months
	now := time.Now()
	firstDayOfThisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	months := []string{}
	for i := 0; i < 4; i++ {
		firstDayOfPastMonth := firstDayOfThisMonth.AddDate(0, -i, 0)
		months = append(months, firstDayOfPastMonth.Format("2006-01-02"))
	}
	firstDayOfNextMonth := firstDayOfThisMonth.AddDate(0, 1, 0)
	costExplorerTimePeriod := &costexplorer.DateInterval{
		Start: aws.String(months[len(months)-1]),
		End:   aws.String(firstDayOfNextMonth.Format("2006-01-02")),
	}
	fmt.Println(costExplorerTimePeriod)

	// Get costs from AWS Cost Explorer
	// Equivalent to getting all pages of:
	// aws ce get-cost-and-usage \
	//   --time-period Start=2019-04-01,End=2019-04-30 \
	//   --granularity MONTHLY \
	//   --filter '{"Not": {"Tags": {"Key": "tenant", "Values": [""]}}}' \
	//   --metrics UnblendedCost \
	//   --group-by Type=TAG,Key=chargeable_entity
	costExplorer := costexplorer.New(session.New())
	costAndUsageInput := costexplorer.GetCostAndUsageInput{
		Filter: &costexplorer.Expression{
			Not: &costexplorer.Expression{
				Tags: &costexplorer.TagValues{
					Key:    aws.String("tenant"),
					Values: []*string{aws.String("")},
				},
			},
		},
		Granularity: aws.String(costexplorer.GranularityMonthly),
		GroupBy: []*costexplorer.GroupDefinition{&costexplorer.GroupDefinition{
			Key:  aws.String("chargeable_entity"),
			Type: aws.String(costexplorer.GroupDefinitionTypeTag),
		}},
		Metrics:    []*string{aws.String(costexplorer.MetricUnblendedCost)},
		TimePeriod: costExplorerTimePeriod,
	}
	costAndUsageResultsByTime, err := costexplorerCostAndUsageByTime(costExplorer, costAndUsageInput)
	if err != nil {
		log.Fatalln(err)
	}

	// Attribute costs to S3 service instances
	monthCostsByServiceInstance := map[string]map[string]string{}
	for _, costAndUsageResultByTime := range costAndUsageResultsByTime {
		monthStartDate := *costAndUsageResultByTime.TimePeriod.Start

		for _, group := range costAndUsageResultByTime.Groups {
			serviceInstanceGuid := strings.TrimPrefix(*group.Keys[0], "chargeable_entity$")
			unblendedCostAmount := fmt.Sprintf("%s %s", *group.Metrics["UnblendedCost"].Amount, *group.Metrics["UnblendedCost"].Unit)
			if _, ok := monthCostsByServiceInstance[serviceInstanceGuid]; !ok {
				monthCostsByServiceInstance[serviceInstanceGuid] = map[string]string{}
			}
			monthCostsByServiceInstance[serviceInstanceGuid][monthStartDate] = unblendedCostAmount
		}
	}

	// Output the table
	fmt.Printf("\nService instances with an empty NAME and ORG were found in AWS Cost Explorer but not in Cloud Foundry. Commonly this means they have been deleted.\n\n")

	headers := []string{"SERVICE INSTANCE GUID", "NAME", "ORG"}
	headers = append(headers, months...)

	data := [][]string{}
	for serviceInstanceGuid, monthlyCosts := range monthCostsByServiceInstance {
		row := []string{serviceInstanceGuid, "", ""}

		// Use metadata from CF if this service instance exists in Cloud Controller
		if serviceInstance, ok := serviceInstances[serviceInstanceGuid]; ok {
			row[1] = serviceInstance.Name
			space := spaces[serviceInstance.SpaceGuid]
			org := orgs[space.OrganizationGuid]
			row[2] = org.Name
		}

		for _, month := range months {
			cost := monthlyCosts[month]
			row = append(row, cost)
		}

		data = append(data, row)
	}
	sort.Slice(data, func(i, j int) bool { return data[i][0] < data[j][0] })

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader(headers)
	for _, r := range data {
		table.Append(r)
	}
	table.Render()
}

func cfClient(apiUrl, token string) (*cfclient.Client, error) {
	// Workaround an undocumented expectation of `go-cfclient`: it doesn't
	// work if `bearer ` is still in the OAuth2 token
	// https://github.com/cloudfoundry-community/go-cfclient/blob/0b4a58fd/client.go#L200
	token = strings.TrimPrefix(token, "bearer ")
	c := &cfclient.Config{
		ApiAddress: apiUrl,
		Token:      token,
	}
	return cfclient.NewClient(c)
}

func cfGetServiceInstancesByServiceLabel(cf *cfclient.Client, serviceLabel string) (map[string]cfclient.ServiceInstance, error) {
	// Get the S3 service
	v := url.Values{}
	v.Set("q", fmt.Sprintf("label:%s", serviceLabel))
	services, err := cf.ListServicesByQuery(v)
	if err != nil {
		return nil, err
	}
	if len(services) != 1 {
		return nil, fmt.Errorf("expected one 'aws-s3-bucket' services: %#v", services)
	}
	service := services[0]

	// Get every service plan for the S3 service
	//log.Printf("fetching service plans of service '%s' '%s'\n", service.Label, service.Guid)
	v = url.Values{}
	v.Set("q", fmt.Sprintf("service_guid:%s", service.Guid))
	servicePlans, err := cf.ListServicePlansByQuery(v)
	if err != nil {
		return nil, err
	}

	// Get the service instances for every S3 service plan
	serviceInstancesSlice := []cfclient.ServiceInstance{}
	for _, servicePlan := range servicePlans {
		//log.Printf("fetching service instances of service plan '%s' '%s'\n", servicePlan.Name, servicePlan.Guid)
		v = url.Values{}
		v.Set("q", fmt.Sprintf("service_plan_guid:%s", servicePlan.Guid))
		servicePlanInstances, err := cf.ListServiceInstancesByQuery(v)
		if err != nil {
			return nil, err
		}
		serviceInstancesSlice = append(serviceInstancesSlice, servicePlanInstances...)
	}

	serviceInstances := map[string]cfclient.ServiceInstance{}
	for _, serviceInstance := range serviceInstancesSlice {
		serviceInstances[serviceInstance.Guid] = serviceInstance
	}
	return serviceInstances, nil
}

func cfGetSpacesAndOrgsFromServiceInstances(cf *cfclient.Client, serviceInstances map[string]cfclient.ServiceInstance) (map[string]cfclient.Org, map[string]cfclient.Space, error) {
	spaces := map[string]cfclient.Space{}
	for _, serviceInstance := range serviceInstances {
		spaceGuid := serviceInstance.SpaceGuid
		if _, ok := spaces[spaceGuid]; ok {
			continue
		}
		space, err := cf.GetSpaceByGuid(spaceGuid)
		if err != nil {
			return nil, nil, err
		}
		spaces[spaceGuid] = space
	}

	orgs := map[string]cfclient.Org{}
	for _, space := range spaces {
		orgGuid := space.OrganizationGuid
		if _, ok := orgs[orgGuid]; ok {
			continue
		}
		org, err := cf.GetOrgByGuid(orgGuid)
		if err != nil {
			return nil, nil, err
		}
		orgs[orgGuid] = org
	}

	return orgs, spaces, nil
}

func costexplorerCostAndUsageByTime(costExplorer *costexplorer.CostExplorer, costAndUsageInput costexplorer.GetCostAndUsageInput) ([]*costexplorer.ResultByTime, error) {
	costAndUsageResultsByTime := []*costexplorer.ResultByTime{}
	var nextPageToken *string
	for {
		costAndUsageInput.NextPageToken = nextPageToken

		if err := costAndUsageInput.Validate(); err != nil {
			return nil, err
		}
		req, costAndUsageOutput := costExplorer.GetCostAndUsageRequest(&costAndUsageInput)
		if err := req.Send(); err != nil {
			return nil, err
		}
		costAndUsageResultsByTime = append(costAndUsageResultsByTime, costAndUsageOutput.ResultsByTime...)

		nextPageToken = costAndUsageOutput.NextPageToken
		if nextPageToken == nil {
			break
		}
	}
	return costAndUsageResultsByTime, nil
}
