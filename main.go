package main

import (
	"context"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
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
		if instance.Status == types.ContainerInstanceStatusDraining && instance.RunningTasksCount < 3 {
			drainingHosts = append(drainingHosts, *instance.Ec2InstanceId)
		}
	}

	return drainingHosts, nil
}

func main() {
	clusterName := "your-cluster-name"
	drainingHosts, err := findDrainingHostsWithFewTasks(clusterName)
	if err != nil {
		log.Fatalf("Error finding draining hosts: %v", err)
	}

	fmt.Println("Draining hosts with fewer than 3 running tasks:")
	for _, host := range drainingHosts {
		fmt.Println(host)
	}
}
