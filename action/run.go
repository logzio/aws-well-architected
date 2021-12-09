package main

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go/logging"
)

type S3 struct {
	ctx    context.Context
	logger logging.Logger
	client *s3.Client
}

const (
	autoDeploymentFileName = "auto-deployment.yaml"
	zipFileName = "lambda_function.zip"
	uploadDirectoryPath = "../upload/"
)

func NewS3() (*S3, error) {
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("error loading default config: %v", err)
	}

	client := s3.NewFromConfig(cfg)

	return &S3{
		ctx:    ctx,
		logger: cfg.Logger,
		client: client,
	}, nil
}

func (s *S3) uploadToAWS(bucket *string, key *string, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}

	defer closeFile(file, &err)

	putObjectInput := &s3.PutObjectInput{
		Bucket: bucket,
		Key: key,
		ACL: types.ObjectCannedACLPublicRead,
		Body: file,
	}
	_, err = s.client.PutObject(s.ctx, putObjectInput)
	if err != nil {
		return fmt.Errorf("error putting file into S3 bucket: %v", err)
	}

	s.logger.Logf("INFO", filePath, " was uploaded successfully into S3 bucket")

	return nil
}

func closeFile(file *os.File, err *error) {
	closeError := file.Close()

	if closeError != nil {
		*err = fmt.Errorf("error closing the file %s: %v", file.Name(), closeError)
	}
}

func main() {
	s, err := NewS3()
	if err != nil {
		panic(err)
	}

	bucketName := os.Getenv("BUCKET_NAME")
	bucketDirectory := os.Getenv("BUCKET_DIR")
	autoDeploymentKey := bucketDirectory + "/" + autoDeploymentFileName
	zipKey := bucketDirectory + "/" + zipFileName

	err = s.uploadToAWS(&bucketName, &autoDeploymentKey, uploadDirectoryPath + autoDeploymentFileName)
	if err != nil {
		panic(err)
	}

	err = s.uploadToAWS(&bucketName, &zipKey, uploadDirectoryPath + zipFileName)
	if err != nil {
		panic(err)
	}
}