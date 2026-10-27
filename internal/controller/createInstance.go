package controller

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"

	computev1 "github.com/Mohdcode/ec2Operator/api/v1"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func createEc2Instance(ctx context.Context, ec2Instance *computev1.Ec2Instance) (createdInstanceInfo *computev1.CreatedInstanceInfo, err error) {
	l := log.Log.WithName("createEc2Instance")

	l.Info("=== STARTING EC2 INSTANCE CREATION ===",
		"ami", ec2Instance.Spec.AMIId,
		"instanceType", ec2Instance.Spec.InstanceType,
		"region", ec2Instance.Spec.Region)

	// create the client for ec2 instance
	ec2Client, err := awsClient(ctx, ec2Instance.Spec.Region)
	if err != nil {
		return nil, err
	}

	// create the input for the run instances
	runInput := &ec2.RunInstancesInput{
		ImageId:      aws.String(ec2Instance.Spec.AMIId),
		InstanceType: ec2types.InstanceType(ec2Instance.Spec.InstanceType),
		MinCount:     aws.Int32(1),
		MaxCount:     aws.Int32(1),
	}
	if ec2Instance.Spec.KeyPair != "" {
		runInput.KeyName = aws.String(ec2Instance.Spec.KeyPair)
	}
	if ec2Instance.Spec.Subnet != "" && !ec2Instance.Spec.AssociatePublicIP {
		runInput.SubnetId = aws.String(ec2Instance.Spec.Subnet)
	}
	if len(ec2Instance.Spec.SecurityGroups) > 0 && !ec2Instance.Spec.AssociatePublicIP {
		runInput.SecurityGroupIds = ec2Instance.Spec.SecurityGroups
	}
	if ec2Instance.Spec.AvailabilityZone != "" {
		runInput.Placement = &ec2types.Placement{AvailabilityZone: aws.String(ec2Instance.Spec.AvailabilityZone)}
	}
	if ec2Instance.Spec.UserData != "" {
		runInput.UserData = aws.String(base64.StdEncoding.EncodeToString([]byte(ec2Instance.Spec.UserData)))
	}
	if ec2Instance.Spec.AssociatePublicIP {
		interfaceSpec := ec2types.InstanceNetworkInterfaceSpecification{
			DeviceIndex:              aws.Int32(0),
			AssociatePublicIpAddress: aws.Bool(true),
		}
		if ec2Instance.Spec.Subnet != "" {
			interfaceSpec.SubnetId = aws.String(ec2Instance.Spec.Subnet)
		}
		interfaceSpec.Groups = ec2Instance.Spec.SecurityGroups
		runInput.NetworkInterfaces = []ec2types.InstanceNetworkInterfaceSpecification{interfaceSpec}
	}
	if len(ec2Instance.Spec.Tags) > 0 {
		keys := make([]string, 0, len(ec2Instance.Spec.Tags))
		for key := range ec2Instance.Spec.Tags {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		tags := make([]ec2types.Tag, 0, len(keys))
		for _, key := range keys {
			tags = append(tags, ec2types.Tag{Key: aws.String(key), Value: aws.String(ec2Instance.Spec.Tags[key])})
		}
		runInput.TagSpecifications = []ec2types.TagSpecification{{
			ResourceType: ec2types.ResourceTypeInstance,
			Tags:         tags,
		}}
	}
	if ec2Instance.Spec.Storage.RootVolume.Size > 0 {
		root := volumeBlockMapping(ec2Instance.Spec.Storage.RootVolume, "/dev/sda1")
		runInput.BlockDeviceMappings = append(runInput.BlockDeviceMappings, root)
	}
	for _, volume := range ec2Instance.Spec.Storage.AdditionalVolumes {
		if volume.Size > 0 && volume.DeviceName != "" {
			runInput.BlockDeviceMappings = append(runInput.BlockDeviceMappings, volumeBlockMapping(volume, volume.DeviceName))
		}
	}

	l.Info("=== CALLING AWS RunInstances API ===")
	// run the instances
	result, err := ec2Client.RunInstances(ctx, runInput)
	if err != nil {
		l.Error(err, "Failed to create EC2 instance")
		return nil, fmt.Errorf("failed to create EC2 instance: %w", err)
	}

	if result == nil || len(result.Instances) == 0 || result.Instances[0].InstanceId == nil {
		l.Error(nil, "No instances returned in RunInstancesOutput")
		return nil, fmt.Errorf("EC2 RunInstances returned no instance ID")
	}

	// Till here, the instance is created and we have
	// Instance ID, private dns and IP, instance type and image id.
	inst := result.Instances[0]
	l.Info("=== EC2 INSTANCE CREATED SUCCESSFULLY ===", "instanceID", *inst.InstanceId)

	state := "pending"
	if inst.State != nil {
		state = string(inst.State.Name)
	}
	createdInstanceInfo = &computev1.CreatedInstanceInfo{
		InstanceID: *inst.InstanceId,
		State:      state,
		PublicIP:   derefString(inst.PublicIpAddress),
		PrivateIP:  derefString(inst.PrivateIpAddress),
		PublicDNS:  derefString(inst.PublicDnsName),
		PrivateDNS: derefString(inst.PrivateDnsName),
	}

	l.Info("=== EC2 INSTANCE CREATION COMPLETED ===",
		"instanceID", createdInstanceInfo.InstanceID,
		"state", createdInstanceInfo.State,
		"publicIP", createdInstanceInfo.PublicIP)

	return createdInstanceInfo, nil
}

func volumeBlockMapping(volume computev1.VolumeConfig, defaultDeviceName string) ec2types.BlockDeviceMapping {
	deviceName := volume.DeviceName
	if deviceName == "" {
		deviceName = defaultDeviceName
	}
	ebs := &ec2types.EbsBlockDevice{
		VolumeSize: aws.Int32(volume.Size),
		Encrypted:  aws.Bool(volume.Encrypted),
	}
	if volume.Type != "" {
		ebs.VolumeType = ec2types.VolumeType(volume.Type)
	}
	return ec2types.BlockDeviceMapping{DeviceName: aws.String(deviceName), Ebs: ebs}
}

// derefString is a helper function to safely dereference *string
func derefString(s *string) string {
	if s != nil {
		return *s
	}
	return ""
}
