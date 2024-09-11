package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/DataDog/datadog-api-client-go/api/v1/datadog"
)

func findDrainingHostsWithFewTasks(clusterName string) ([]string, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config: %v", err)
	}

	client := ecs.NewFromConfig(cfg)

	input := &ecs.ListContainerInstancesInput{
		Cluster: &clusterName,
	}

	resp, err := client.ListContainerInstances(context.TODO(), input)
	if err != nil {
		return nil, fmt.Errorf("error listing container instances: %v", err)
	}

	describeInput := &ecs.DescribeContainerInstancesInput{
		Cluster:            &clusterName,
		ContainerInstances: resp.ContainerInstanceArns,
	}

	describeResp, err := client.DescribeContainerInstances(context.TODO(), describeInput)
	if err != nil {
		return nil, fmt.Errorf("error describing container instances: %v", err)
	}

	var drainingHosts []string
	for _, instance := range describeResp.ContainerInstances {
		if instance.Status != nil && *instance.Status == "DRAINING" && instance.RunningTasksCount < 3 {
			drainingHosts = append(drainingHosts, *instance.Ec2InstanceId)
		}
	}

	return drainingHosts, nil
}

func terminateEC2Instances(instanceIDs []string) error {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return fmt.Errorf("unable to load SDK config: %v", err)
	}

	client := ec2.NewFromConfig(cfg)

	input := &ec2.TerminateInstancesInput{
		InstanceIds: instanceIDs,
	}

	_, err = client.TerminateInstances(context.TODO(), input)
	if err != nil {
		return fmt.Errorf("error terminating instances: %v", err)
	}

	return nil
}

func findHighDiskUsageHosts(clusterName string) ([]string, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config: %v", err)
	}

	ecsClient := ecs.NewFromConfig(cfg)

	// List container instances in the cluster
	listInput := &ecs.ListContainerInstancesInput{
		Cluster: &clusterName,
	}
	listResp, err := ecsClient.ListContainerInstances(context.TODO(), listInput)
	if err != nil {
		return nil, fmt.Errorf("error listing container instances: %v", err)
	}

	// Describe container instances to get EC2 instance IDs
	describeInput := &ecs.DescribeContainerInstancesInput{
		Cluster:            &clusterName,
		ContainerInstances: listResp.ContainerInstanceArns,
	}
	describeResp, err := ecsClient.DescribeContainerInstances(context.TODO(), describeInput)
	if err != nil {
		return nil, fmt.Errorf("error describing container instances: %v", err)
	}

	// Initialize the Datadog client
	ctx := context.WithValue(
		context.Background(),
		datadog.ContextAPIKeys,
		map[string]datadog.APIKey{
			"apiKeyAuth": {
				Key: "YOUR_DATADOG_API_KEY",
			},
		},
	)

	configuration := datadog.NewConfiguration()
	apiClient := datadog.NewAPIClient(configuration)

	var highDiskUsageHosts []string
	for _, instance := range describeResp.ContainerInstances {
		// Get disk usage metric for the instance from Datadog
		query := fmt.Sprintf("avg:system.disk.in_use{host:%s}", *instance.Ec2InstanceId)
		from := time.Now().Add(-5 * time.Minute).Unix()
		to := time.Now().Unix()

		resp, r, err := apiClient.MetricsApi.QueryMetrics(ctx, from, to, query)
		if err != nil {
			return nil, fmt.Errorf("error querying Datadog metrics: %v", err)
		}
		if r.StatusCode != 200 {
			return nil, fmt.Errorf("received non-200 response from Datadog: %d", r.StatusCode)
		}

		// Check if disk usage is over 85%
		if len(resp.Series) > 0 && len(resp.Series[0].Pointlist) > 0 {
			latestPoint := resp.Series[0].Pointlist[len(resp.Series[0].Pointlist)-1]
			if latestPoint[1] > 0.85 {
				highDiskUsageHosts = append(highDiskUsageHosts, *instance.Ec2InstanceId)
			}
		}
	}

	return highDiskUsageHosts, nil
}

func main() {
	clusterName := "core-production"

	// drainingHosts, err := findDrainingHostsWithFewTasks(clusterName)
	// if err != nil {
	// 	log.Fatalf("Error finding draining hosts: %v", err)
	// }

	// fmt.Println("Draining hosts with fewer than 3 running tasks:")
	// for _, host := range drainingHosts {
	// 	fmt.Println(host)
	// }

	highDiskUsageHosts, err := findHighDiskUsageHosts(clusterName)
	if err != nil {
		log.Fatalf("Error finding hosts with high disk usage: %v", err)
	}

	fmt.Println("\nHosts with more than 85% disk usage:")
	for _, host := range highDiskUsageHosts {
		fmt.Println(host)
	}

	// Commented out termination logic
	// if len(drainingHosts) > 0 {
	// 	fmt.Println("Terminating the following instances:")
	// 	for _, host := range drainingHosts {
	// 		fmt.Println(host)
	// 	}

	// 	err = terminateEC2Instances(drainingHosts)
	// 	if err != nil {
	// 		log.Fatalf("Error terminating instances: %v", err)
	// 	}
	// 	fmt.Println("Instances terminated successfully")
	// } else {
	// 	fmt.Println("No instances to terminate")
	// }
}
