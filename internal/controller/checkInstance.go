package controller

import (
	"context"
	"strings"

	computev1 "github.com/Mohdcode/ec2Operator/api/v1"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func checkEC2InstanceExists(ctx context.Context, instanceID string, ec2Instance *computev1.Ec2Instance) (bool, *ec2types.Instance, error) {
	ec2Client, err := awsClient(ctx, ec2Instance.Spec.Region)
	if err != nil {
		return false, nil, err
	}

	input := &ec2.DescribeInstancesInput{InstanceIds: []string{instanceID}}

	result, err := ec2Client.DescribeInstances(ctx, input)
	if err != nil {
		// Check if it's a "not found" error
		if strings.Contains(err.Error(), "InvalidInstanceID.NotFound") {
			return false, nil, nil
		}
		return false, nil, err
	}

	// Check if we got any instances back
	if result == nil || len(result.Reservations) == 0 || len(result.Reservations[0].Instances) == 0 {
		// No reservations means the instance is not found or not running
		return false, nil, nil
	}
	instance := &result.Reservations[0].Instances[0]
	if instance.State != nil && (instance.State.Name == ec2types.InstanceStateNameTerminated || instance.State.Name == ec2types.InstanceStateNameShuttingDown) {
		return false, nil, nil
	}
	return true, instance, nil
}
