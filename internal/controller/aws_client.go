package controller

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

func awsClient(region string) *ec2.Client {
	accessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, "")),
	)
	if err != nil {
		fmt.Println("Error loading AWS config:", err)
		os.Exit(1)
	}

	// Support for LocalStack: if AWS_ENDPOINT is set, provide it to the client constructor
	if endpoint := os.Getenv("AWS_ENDPOINT"); endpoint != "" {
		return ec2.NewFromConfig(cfg, func(o *ec2.Options) {
			o.BaseEndpoint = aws.String(endpoint)
		})
	}

	return ec2.NewFromConfig(cfg)
}
