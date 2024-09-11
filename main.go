package main

import (
	"context"
	"fmt"
	"log"

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

func main() {
	clusterName := "core-production"
	drainingHosts, err := findDrainingHostsWithFewTasks(clusterName)
	if err != nil {
		log.Fatalf("Error finding draining hosts: %v", err)
	}

	fmt.Println("Draining hosts with fewer than 3 running tasks:")
	for _, host := range drainingHosts {
		fmt.Println(host)
	}

	if len(drainingHosts) > 0 {
		fmt.Println("Terminating the following instances:")
		for _, host := range drainingHosts {
			fmt.Println(host)
		}

		err = terminateEC2Instances(drainingHosts)
		if err != nil {
			log.Fatalf("Error terminating instances: %v", err)
		}
		fmt.Println("Instances terminated successfully")
	} else {
		fmt.Println("No instances to terminate")
	}
}
