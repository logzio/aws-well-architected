package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/wellarchitected"
	"github.com/aws/aws-sdk-go-v2/service/wellarchitected/types"
	"github.com/aws/smithy-go/logging"
	"github.com/logzio/logzio-go"
)

type WellArchitected struct {
	ctx    context.Context
	logger logging.Logger
	client *wellarchitected.Client
	data   [][]byte
}

const (
	maxBulkSizeBytes  = 10 * 1024 * 1024 // 10 MB
	logzioSendingType = "aws-wa"
)

func NewWellArchitected(ctx context.Context) (*WellArchitected, error) {
	cfg, err := config.LoadDefaultConfig(ctx)

	if err != nil {
		return nil, fmt.Errorf("error loading default config: %v", err)
	}

	client := wellarchitected.NewFromConfig(cfg)

	if err != nil {
		return nil, fmt.Errorf("error creating LogzioSender: %v", err)
	}

	return &WellArchitected{
		ctx:    ctx,
		logger: cfg.Logger,
		client: client,
	}, nil
}

func HandleRequest(ctx context.Context) error {
	wellArchitected, err := NewWellArchitected(ctx)

	if err != nil {
		return err
	}

	wellArchitected.logger.Logf("INFO", "Collecting data...")

	err = wellArchitected.collectData()

	if err != nil {
		return err
	}

	wellArchitected.logger.Logf("INFO", "Sending data to Logz.io...")

	err = wellArchitected.sendDataToLogzio()

	if err != nil {
		return err
	}

	wellArchitected.logger.Logf("INFO", "Finished successfully")

	return nil
}

func (wa *WellArchitected) collectData() error {
	workloadSummaries, err := wa.getWorkloadSummaries()

	if err != nil {
		return err
	}

	for _, workloadSummary := range workloadSummaries {
		workloadID := workloadSummary.WorkloadId
		workload, err := wa.getWorkload(workloadID)

		if err != nil {
			return err
		}

		for _, lens := range workload.Lenses {
			lensReview, err := wa.getLensReview(workloadID, &lens)

			if err != nil {
				return err
			}

			pillarReviewSummaries := lensReview.LensReview.PillarReviewSummaries

			for _, pillarReviewSummary := range pillarReviewSummaries {
				err := wa.parseLensReview(lensReview, &pillarReviewSummary)

				if err != nil {
					return err
				}
			}

			lensReviewImprovements, err := wa.getLensReviewImprovements(workloadID, &lens)

			if err != nil {
				return err
			}

			improvementSummaries := lensReviewImprovements.ImprovementSummaries

			for _, improvementSummary := range improvementSummaries {
				err := wa.parseLensReviewImprovements(lensReviewImprovements, &improvementSummary)

				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (wa *WellArchitected) getWorkloadSummaries() ([]types.WorkloadSummary, error) {
	listWorkloadsInput := &wellarchitected.ListWorkloadsInput{}
	listWorkloadsOutput, err := wa.client.ListWorkloads(wa.ctx, listWorkloadsInput)

	if err != nil {
		return nil, fmt.Errorf("did not get workload summaries: %v", err)
	}

	return listWorkloadsOutput.WorkloadSummaries, nil
}

func (wa *WellArchitected) getWorkload(workloadID *string) (*types.Workload, error) {
	workloadInput := &wellarchitected.GetWorkloadInput{WorkloadId: workloadID}
	workloadOutput, err := wa.client.GetWorkload(wa.ctx, workloadInput)

	if err != nil {
		return nil, fmt.Errorf("did not get workload with id %s: %v", *workloadID, err)
	}

	workloadJSON, err := json.Marshal(workloadOutput.Workload)

	if err != nil {
		return nil, fmt.Errorf("error marshaling workload with id %s: %v", *workloadID, err)
	}

	workloadJSON, err = addTypeToData(workloadJSON)

	if err != nil {
		return nil, err
	}

	wa.data = append(wa.data, workloadJSON)

	return workloadOutput.Workload, nil
}

func (wa *WellArchitected) getLensReview(workloadID *string, lensAlias *string) (*wellarchitected.GetLensReviewOutput, error) {
	lensReviewInput := &wellarchitected.GetLensReviewInput{
		WorkloadId: workloadID,
		LensAlias:  lensAlias,
	}

	lensReviewOutput, err := wa.client.GetLensReview(wa.ctx, lensReviewInput)

	if err != nil {
		return nil, fmt.Errorf("did not get lens review with workload id %s and lens alias %s: %v", *workloadID, *lensAlias, err)
	}

	return lensReviewOutput, nil
}

func (wa *WellArchitected) parseLensReview(lensReview *wellarchitected.GetLensReviewOutput, pillarReviewSummary *types.PillarReviewSummary) error {
	lensReviewJSON, err := json.Marshal(lensReview)

	if err != nil {
		return fmt.Errorf("error marshaling lens review with workload id %s and lens alias %s: %v",
			*lensReview.WorkloadId, *lensReview.LensReview.LensAlias, err)
	}

	var lensReviewMap map[string]interface{}
	err = json.Unmarshal(lensReviewJSON, &lensReviewMap)

	if err != nil {
		return fmt.Errorf("error unmarshaling lens review with workload id %s and lens alias %s: %v",
			*lensReview.WorkloadId, *lensReview.LensReview.LensAlias, err)
	}

	delete(lensReviewMap["LensReview"].(map[string]interface{}), "PillarReviewSummaries")

	lensReviewMap["LensReview"].(map[string]interface{})["PillarReviewSummary"] = pillarReviewSummary

	parsedLensReviewJSON, err := json.Marshal(lensReviewMap)

	if err != nil {
		return fmt.Errorf("error marshaling lens review with workload id %s and lens alias %s: %v",
			*lensReview.WorkloadId, *lensReview.LensReview.LensAlias, err)
	}

	parsedLensReviewJSON, err = addTypeToData(parsedLensReviewJSON)

	if err != nil {
		return err
	}

	wa.data = append(wa.data, parsedLensReviewJSON)

	return nil
}

func (wa *WellArchitected) getLensReviewImprovements(workloadID *string, lensAlias *string) (*wellarchitected.ListLensReviewImprovementsOutput, error) {
	lensReviewImprovementsInput := &wellarchitected.ListLensReviewImprovementsInput{
		WorkloadId: workloadID,
		LensAlias:  lensAlias,
	}

	lensReviewImprovementsOutput, err := wa.client.ListLensReviewImprovements(wa.ctx, lensReviewImprovementsInput)

	if err != nil {
		return nil, fmt.Errorf("did not get lens review improvements with workload id %s and lens alias %s: %v", *workloadID, *lensAlias, err)
	}

	return lensReviewImprovementsOutput, nil
}

func (wa *WellArchitected) parseLensReviewImprovements(
	lensReviewImprovements *wellarchitected.ListLensReviewImprovementsOutput,
	improvementSummary *types.ImprovementSummary,
) error {
	lensReviewImprovementsJSON, err := json.Marshal(lensReviewImprovements)

	if err != nil {
		return fmt.Errorf("error marshaling lens review improvements with workload id %s and lens alias %s: %v",
			*lensReviewImprovements.WorkloadId, *lensReviewImprovements.LensAlias, err)
	}

	var lensReviewImprovementsMap map[string]interface{}
	err = json.Unmarshal(lensReviewImprovementsJSON, &lensReviewImprovementsMap)

	if err != nil {
		return fmt.Errorf("error unmarshaling lens review improvements with workload id %s and lens alias %s: %v",
			*lensReviewImprovements.WorkloadId, *lensReviewImprovements.LensAlias, err)
	}

	delete(lensReviewImprovementsMap, "ImprovementSummaries")

	lensReviewImprovementsMap["ImprovementSummary"] = improvementSummary

	parsedLensReviewJSON, err := json.Marshal(lensReviewImprovementsMap)

	if err != nil {
		return fmt.Errorf("error marshaling lens review improvements with workload id %s and lens alias %s: %v",
			*lensReviewImprovements.WorkloadId, *lensReviewImprovements.LensAlias, err)
	}

	parsedLensReviewJSON, err = addTypeToData(parsedLensReviewJSON)

	if err != nil {
		return err
	}

	wa.data = append(wa.data, parsedLensReviewJSON)

	return nil
}

func (wa *WellArchitected) sendDataToLogzio() error {
	logzioSender, err := logzio.New(
		os.Getenv("LOGZIO_TOKEN"),
		logzio.SetUrl(os.Getenv("LOGZIO_URL")),
		logzio.SetInMemoryQueue(true),
		logzio.SetinMemoryCapacity(maxBulkSizeBytes),
	)

	if err != nil {
		return fmt.Errorf("error creating LogzioSender: %v", err)
	}

	for _, data := range wa.data {
		err := logzioSender.Send(data)

		if err != nil {
			return fmt.Errorf("error sending data to Logz.io: %v", err)
		}
	}

	logzioSender.Stop()

	return nil
}

func addTypeToData(data []byte) ([]byte, error) {
	var dataWithType map[string]interface{}

	err := json.Unmarshal(data, &dataWithType)

	if err != nil {
		return nil, err
	}

	dataWithType["type"] = logzioSendingType

	dataWithTypeJSON, err := json.Marshal(dataWithType)

	if err != nil {
		return nil, err
	}

	return dataWithTypeJSON, nil
}

func main() {
	lambda.Start(HandleRequest)
}
