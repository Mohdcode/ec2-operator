package controller

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

func awsClient(ctx context.Context, region string) (*ec2.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("load AWS configuration: %w", err)
	}

	// AWS_ENDPOINT is optional and supports AWS-compatible local development services.
	if endpoint := os.Getenv("AWS_ENDPOINT"); endpoint != "" {
		return ec2.NewFromConfig(cfg, func(o *ec2.Options) {
			o.BaseEndpoint = aws.String(endpoint)
		}), nil
	}

	return ec2.NewFromConfig(cfg), nil
}
