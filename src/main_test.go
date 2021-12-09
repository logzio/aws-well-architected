package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/wellarchitected"
	"github.com/aws/aws-sdk-go-v2/service/wellarchitected/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	workloadName        = "Workload-Test"
	workloadDescription = "Integration aws-well-architected tests"
	workloadReviewOwner = "Logz.io"
	awsRegion           = "us-east-1"
	lensName            = "wellarchitected"
	logzioURL           = "https://listener.logz.io:8071"
	logzioToken         = "123456789a"
)

var (
	wa             *WellArchitected
	testWorkloadID *string
)

func TestMain(m *testing.M) {
	err := setup()
	if err != nil {
		panic(err)
	}

	code := m.Run()

	err = teardown()
	if err != nil {
		panic(err)
	}

	os.Exit(code)
}

func setup() error {
	err := os.Setenv("LOGZIO_URL", logzioURL)
	if err != nil {
		return fmt.Errorf("got error setting logzio url environment variable. error: %v", err)
	}

	err = os.Setenv("LOGZIO_TOKEN", logzioToken)
	if err != nil {
		return fmt.Errorf("got error setting logzio token environment variable. error: %v", err)
	}

	ctx := context.Background()
	wa, err = NewWellArchitected(ctx)
	if err != nil {
		return err
	}

	err = createWorkload()
	if err != nil {
		return err
	}

	return nil
}

func createWorkload() error {
	name := workloadName
	description := workloadDescription
	reviewOwner := workloadReviewOwner
	workloadInput := &wellarchitected.CreateWorkloadInput{
		WorkloadName: &name,
		Description:  &description,
		Environment:  types.WorkloadEnvironmentPreproduction,
		AwsRegions:   []string{awsRegion},
		ReviewOwner:  &reviewOwner,
		Lenses:       []string{lensName},
	}
	workloadOutput, err := wa.client.CreateWorkload(wa.ctx, workloadInput)
	if err != nil {
		return fmt.Errorf("got error creating the workload. error: %v", err)
	}

	testWorkloadID = workloadOutput.WorkloadId

	return nil
}

func teardown() error {
	workloadInput := &wellarchitected.DeleteWorkloadInput{
		WorkloadId: testWorkloadID,
	}
	_, err := wa.client.DeleteWorkload(wa.ctx, workloadInput)
	if err != nil {
		return fmt.Errorf("got error deleting the workload. error: %v", err)
	}

	return nil
}

func resetWellArchitected() {
	wa.data = make([][]byte, 0)
}

func TestCollectData_Success(t *testing.T) {
	err := wa.collectData()
	require.NoError(t, err)

	lens := lensName
	lensReview, err := wa.getLensReview(testWorkloadID, &lens)
	require.NoError(t, err)
	require.NotNil(t, lensReview)

	lensReviewImprovements, err := wa.getLensReviewImprovements(testWorkloadID, &lens)
	require.NoError(t, err)
	require.NotNil(t, lensReviewImprovements)

	lensReviewsNum := len(lensReview.LensReview.PillarReviewSummaries)
	lensReviewImprovementsNum := len(lensReviewImprovements.ImprovementSummaries)
	dataCounter := 0

	for _, data := range wa.data {
		if strings.Contains(string(data), *testWorkloadID) {
			dataCounter++
		}
	}

	assert.Equal(t, lensReviewsNum+lensReviewImprovementsNum+1, dataCounter)

	resetWellArchitected()
}

func TestSendingDataToLogzio_Success(t *testing.T) {
	err := wa.collectData()
	require.NoError(t, err)

	err = wa.sendDataToLogzio()
	require.NoError(t, err)

	resetWellArchitected()
}

func TestHandleRequest_Success(t *testing.T) {
	err := HandleRequest(wa.ctx)
	require.NoError(t, err)

	resetWellArchitected()
}
