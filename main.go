package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/DataDog/datadog-api-client-go/api/v1/datadog"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
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
	// Initialize the Datadog client
	ctx := context.WithValue(
		context.Background(),
		datadog.ContextAPIKeys,
		map[string]datadog.APIKey{
			"apiKeyAuth": {
				Key: os.Getenv("DD_CLIENT_API_KEY"),
			},
			"appKeyAuth": {
				Key: os.Getenv("DD_CLIENT_APP_KEY"),
			},
		},
	)
	configuration := datadog.NewConfiguration()
	apiClient := datadog.NewAPIClient(configuration)

	// Construct the query for all hosts in the cluster
	query := fmt.Sprintf("max:system.disk.in_use{account_name:production,ecs_cluster:%s} by {host}", clusterName)
	from := time.Now().Add(-5 * time.Minute).Unix()
	to := time.Now().Unix()

	// Query Datadog for metrics
	resp, r, err := apiClient.MetricsApi.QueryMetrics(ctx, from, to, query)
	if err != nil {
		return nil, fmt.Errorf("error querying Datadog metrics: %v", err)
	}
	if r.StatusCode != 200 {
		return nil, fmt.Errorf("received non-200 response from Datadog: %d", r.StatusCode)
	}

	var highDiskUsageHosts []string
	for _, series := range resp.Series {
		if len(series.Pointlist) > 0 {
			latestPoint := series.Pointlist[len(series.Pointlist)-1]
			hostTags := series.Scope
			var hostID string
			for _, tag := range hostTags {
				if strings.HasPrefix(tag, "host:") {
					hostID = strings.TrimPrefix(tag, "host:")
					break
				}
			}
			fmt.Printf("%v %v\n", hostID, *latestPoint[1])
			if *latestPoint[1] > 0.85 {
				highDiskUsageHosts = append(highDiskUsageHosts, hostID)
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
