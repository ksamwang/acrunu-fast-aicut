package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
)

func newScriptGenerationJobTestService(t *testing.T, generator modelgateway.ScriptGenerator) (*ScriptGenerationJobService, Product, SellingPoint) {
	t.Helper()
	products := NewProductAssetService()
	product := products.CreateProduct(CreateProductInput{Name: "束裤带", Description: "骑行裤脚固定用品"})
	point, err := products.CreateSellingPoint(product.ID, CreateSellingPointInput{Title: "避免蹭链条", Priority: 1})
	if err != nil {
		t.Fatalf("create selling point: %v", err)
	}
	scripts := NewScriptGenerationService(products, NewSystemConfigService(), NewModelProviderService(), config.Config{}).WithGenerator(generator)
	return NewScriptGenerationJobService(nil, scripts), product, point
}

func createScriptGenerationJobForTest(t *testing.T, service *ScriptGenerationJobService, userID string, product Product, point SellingPoint) ScriptGenerationJob {
	t.Helper()
	job, err := service.Create(context.Background(), CreateScriptGenerationJobInput{
		CreatedByUserID: userID,
		Mode:            ScriptGenerationJobModeReplaceAll,
		BaseRevision:    "draft-v1",
		GenerationInput: WorkbenchScriptGenerationInput{
			ProductID:       product.ID,
			SellingPointIDs: []string{point.ID},
			VariantCount:    1,
		},
	})
	if err != nil {
		t.Fatalf("create script generation job: %v", err)
	}
	return job
}

func TestScriptGenerationJobLifecyclePersistsResult(t *testing.T) {
	generator := &recordingScriptGenerator{result: validScriptGenerationResult("避免蹭链条", "避免蹭链条")}
	service, product, point := newScriptGenerationJobTestService(t, generator)
	userID := uuid.NewString()
	job := createScriptGenerationJobForTest(t, service, userID, product, point)
	if job.Input.Temperature == nil || *job.Input.Temperature != modelgateway.DefaultScriptGenerationTemperature {
		t.Fatalf("expected job input to persist default temperature, got %#v", job.Input.Temperature)
	}

	if err := service.Process(context.Background(), job.ID); err != nil {
		t.Fatalf("process script generation job: %v", err)
	}
	completed, err := service.GetForUser(context.Background(), job.ID, userID)
	if err != nil {
		t.Fatalf("get completed job: %v", err)
	}
	if completed.Status != ScriptGenerationJobStatusCompleted || len(completed.ResultVariants) != 1 {
		t.Fatalf("unexpected completed job %#v", completed)
	}
	latest, err := service.GetLatestUnresolvedForUser(context.Background(), userID)
	if err != nil || latest.ID != job.ID {
		t.Fatalf("expected latest unresolved job %s, got %#v err=%v", job.ID, latest, err)
	}
	resolved, err := service.ResolveForUser(context.Background(), job.ID, userID, ScriptGenerationJobStatusApplied)
	if err != nil || resolved.Status != ScriptGenerationJobStatusApplied {
		t.Fatalf("resolve completed job: %#v err=%v", resolved, err)
	}
	if _, err := service.GetLatestUnresolvedForUser(context.Background(), userID); !errors.Is(err, ErrScriptGenerationJobNotFound) {
		t.Fatalf("expected no unresolved job, got %v", err)
	}
}

func TestScriptGenerationJobLimitsActiveJobsAndIsolatesUsers(t *testing.T) {
	generator := &recordingScriptGenerator{result: validScriptGenerationResult("避免蹭链条", "避免蹭链条")}
	service, product, point := newScriptGenerationJobTestService(t, generator)
	firstUserID := uuid.NewString()
	secondUserID := uuid.NewString()
	job := createScriptGenerationJobForTest(t, service, firstUserID, product, point)

	_, err := service.Create(context.Background(), CreateScriptGenerationJobInput{
		CreatedByUserID: firstUserID,
		Mode:            ScriptGenerationJobModeReplaceAll,
		BaseRevision:    "draft-v2",
		GenerationInput: WorkbenchScriptGenerationInput{ProductID: product.ID, SellingPointIDs: []string{point.ID}, VariantCount: 1},
	})
	if !errors.Is(err, ErrScriptGenerationJobActive) {
		t.Fatalf("expected active job conflict, got %v", err)
	}
	if _, err := service.GetForUser(context.Background(), job.ID, secondUserID); !errors.Is(err, ErrScriptGenerationJobNotFound) {
		t.Fatalf("expected user isolation, got %v", err)
	}
	if _, err := service.Create(context.Background(), CreateScriptGenerationJobInput{
		CreatedByUserID: secondUserID,
		Mode:            ScriptGenerationJobModeReplaceAll,
		BaseRevision:    "draft-v1",
		GenerationInput: WorkbenchScriptGenerationInput{ProductID: product.ID, SellingPointIDs: []string{point.ID}, VariantCount: 1},
	}); err != nil {
		t.Fatalf("second user should be able to create a job: %v", err)
	}
}

type blockingScriptGenerator struct {
	started chan struct{}
	release chan struct{}
	result  modelgateway.ScriptGenerationResult
}

type progressiveScriptGenerator struct {
	firstCompleted chan struct{}
	release        chan struct{}
}

func (g *progressiveScriptGenerator) GenerateScripts(context.Context, modelgateway.ScriptGenerationInput) (modelgateway.ScriptGenerationResult, error) {
	return modelgateway.ScriptGenerationResult{}, errors.New("progressive generator must use GenerateScriptsWithProgress")
}

func (g *progressiveScriptGenerator) GenerateScriptsWithProgress(ctx context.Context, _ modelgateway.ScriptGenerationInput, onProgress modelgateway.ScriptGenerationProgressHandler) (modelgateway.ScriptGenerationResult, error) {
	copies := []modelgateway.ScriptCopyVariant{
		{VariantIndex: 1, Hook: "第一条", ScriptText: "第一条，展示完整产品功能和实际使用效果。"},
		{VariantIndex: 2, Hook: "第二条", ScriptText: "第二条，展示另一种产品功能和实际使用效果。"},
	}
	for _, copyVariant := range copies {
		if err := onProgress(modelgateway.ScriptGenerationVariantProgress{
			VariantIndex: copyVariant.VariantIndex,
			Status:       modelgateway.ScriptGenerationProgressGenerating,
			Copy:         copyVariant,
		}); err != nil {
			return modelgateway.ScriptGenerationResult{}, err
		}
	}
	completed := modelgateway.ScriptGenerationVariant{
		Hook:          copies[0].Hook,
		ScriptText:    copies[0].ScriptText,
		EditingIntent: "展示产品使用过程",
		Beats: []modelgateway.ScriptGenerationBeat{{
			Label: "产品使用", SellingPoint: "避免蹭链条", VisualGoal: "人物操作产品并展示使用状态", SourceType: modelgateway.TTSVisualSourceType,
		}},
	}
	if err := onProgress(modelgateway.ScriptGenerationVariantProgress{
		VariantIndex: 1,
		Status:       modelgateway.ScriptGenerationProgressCompleted,
		Copy:         copies[0],
		Variant:      completed,
	}); err != nil {
		return modelgateway.ScriptGenerationResult{}, err
	}
	close(g.firstCompleted)
	select {
	case <-g.release:
	case <-ctx.Done():
		return modelgateway.ScriptGenerationResult{}, ctx.Err()
	}
	if err := onProgress(modelgateway.ScriptGenerationVariantProgress{
		VariantIndex: 2,
		Status:       modelgateway.ScriptGenerationProgressFailed,
		Copy:         copies[1],
		Err:          errors.New("invalid visual intent"),
	}); err != nil {
		return modelgateway.ScriptGenerationResult{}, err
	}
	return modelgateway.ScriptGenerationResult{Variants: []modelgateway.ScriptGenerationVariant{completed}}, nil
}

func TestScriptGenerationJobPublishesEachVariantProgressively(t *testing.T) {
	generator := &progressiveScriptGenerator{firstCompleted: make(chan struct{}), release: make(chan struct{})}
	service, product, point := newScriptGenerationJobTestService(t, generator)
	userID := uuid.NewString()
	job, err := service.Create(context.Background(), CreateScriptGenerationJobInput{
		CreatedByUserID: userID,
		Mode:            ScriptGenerationJobModeReplaceAll,
		BaseRevision:    "draft-v1",
		GenerationInput: WorkbenchScriptGenerationInput{
			ProductID:       product.ID,
			SellingPointIDs: []string{point.ID},
			VariantCount:    2,
		},
	})
	if err != nil {
		t.Fatalf("create script generation job: %v", err)
	}
	processResult := make(chan error, 1)
	go func() { processResult <- service.Process(context.Background(), job.ID) }()
	<-generator.firstCompleted

	inProgress, err := service.GetForUser(context.Background(), job.ID, userID)
	if err != nil {
		t.Fatalf("get in-progress job: %v", err)
	}
	if inProgress.Status != ScriptGenerationJobStatusGenerating || len(inProgress.ResultVariants) != 2 {
		t.Fatalf("expected two persisted progress slots, got %#v", inProgress)
	}
	if inProgress.ResultVariants[0].Status != "draft" || inProgress.ResultVariants[1].Status != "generating" {
		t.Fatalf("unexpected progressive statuses %#v", inProgress.ResultVariants)
	}

	close(generator.release)
	if err := <-processResult; err != nil {
		t.Fatalf("process progressive job: %v", err)
	}
	completed, err := service.GetForUser(context.Background(), job.ID, userID)
	if err != nil {
		t.Fatalf("get completed job: %v", err)
	}
	if completed.Status != ScriptGenerationJobStatusCompleted || len(completed.ResultVariants) != 2 {
		t.Fatalf("unexpected completed progressive job %#v", completed)
	}
	if completed.ResultVariants[0].Status != "draft" || completed.ResultVariants[1].Status != "failed" || completed.ResultVariants[1].ErrorMessage == "" {
		t.Fatalf("expected successful and failed slots to coexist, got %#v", completed.ResultVariants)
	}
}

func (g *blockingScriptGenerator) GenerateScripts(ctx context.Context, _ modelgateway.ScriptGenerationInput) (modelgateway.ScriptGenerationResult, error) {
	close(g.started)
	select {
	case <-g.release:
		return g.result, nil
	case <-ctx.Done():
		return modelgateway.ScriptGenerationResult{}, ctx.Err()
	}
}

func TestScriptGenerationJobCancellationDoesNotPublishLateResult(t *testing.T) {
	generator := &blockingScriptGenerator{
		started: make(chan struct{}),
		release: make(chan struct{}),
		result:  validScriptGenerationResult("避免蹭链条", "避免蹭链条"),
	}
	service, product, point := newScriptGenerationJobTestService(t, generator)
	userID := uuid.NewString()
	job := createScriptGenerationJobForTest(t, service, userID, product, point)
	processResult := make(chan error, 1)
	go func() {
		processResult <- service.Process(context.Background(), job.ID)
	}()
	<-generator.started

	cancelled, err := service.CancelForUser(context.Background(), job.ID, userID)
	if err != nil || cancelled.Status != ScriptGenerationJobStatusCancelled {
		t.Fatalf("cancel generating job: %#v err=%v", cancelled, err)
	}
	close(generator.release)
	if err := <-processResult; err != nil {
		t.Fatalf("late generator completion should be ignored: %v", err)
	}
	stored, err := service.GetForUser(context.Background(), job.ID, userID)
	if err != nil {
		t.Fatalf("get cancelled job: %v", err)
	}
	if stored.Status != ScriptGenerationJobStatusCancelled || len(stored.ResultVariants) != 0 {
		t.Fatalf("late result was published after cancellation: %#v", stored)
	}
}

func TestScriptGenerationJobResumesGeneratingState(t *testing.T) {
	generator := &recordingScriptGenerator{result: validScriptGenerationResult("避免蹭链条", "避免蹭链条")}
	service, product, point := newScriptGenerationJobTestService(t, generator)
	userID := uuid.NewString()
	job := createScriptGenerationJobForTest(t, service, userID, product, point)
	if err := service.markGenerating(context.Background(), job.ID); err != nil {
		t.Fatalf("mark job generating: %v", err)
	}

	if err := service.Process(context.Background(), job.ID); err != nil {
		t.Fatalf("resume generating job: %v", err)
	}
	stored, err := service.GetForUser(context.Background(), job.ID, userID)
	if err != nil || stored.Status != ScriptGenerationJobStatusCompleted {
		t.Fatalf("unexpected resumed job %#v err=%v", stored, err)
	}
}

func TestScriptGenerationJobReturnsToQueueWhenWorkerContextStops(t *testing.T) {
	generator := &blockingScriptGenerator{
		started: make(chan struct{}),
		release: make(chan struct{}),
		result:  validScriptGenerationResult("避免蹭链条", "避免蹭链条"),
	}
	service, product, point := newScriptGenerationJobTestService(t, generator)
	userID := uuid.NewString()
	job := createScriptGenerationJobForTest(t, service, userID, product, point)
	ctx, cancel := context.WithCancel(context.Background())
	processResult := make(chan error, 1)
	go func() {
		processResult <- service.Process(ctx, job.ID)
	}()
	<-generator.started
	cancel()
	if err := <-processResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancelled worker context, got %v", err)
	}
	stored, err := service.GetForUser(context.Background(), job.ID, userID)
	if err != nil || stored.Status != ScriptGenerationJobStatusQueued {
		t.Fatalf("expected job to return to queue, got %#v err=%v", stored, err)
	}
}

func TestScriptGenerationJobListsPendingJobs(t *testing.T) {
	generator := &recordingScriptGenerator{result: validScriptGenerationResult("避免蹭链条", "避免蹭链条")}
	service, product, point := newScriptGenerationJobTestService(t, generator)
	userID := uuid.NewString()
	job := createScriptGenerationJobForTest(t, service, userID, product, point)

	ids, err := service.PendingJobIDs(context.Background())
	if err != nil || len(ids) != 1 || ids[0] != job.ID {
		t.Fatalf("unexpected pending jobs %v err=%v", ids, err)
	}
	if _, err := service.CancelForUser(context.Background(), job.ID, userID); err != nil {
		t.Fatalf("cancel pending job: %v", err)
	}
	ids, err = service.PendingJobIDs(context.Background())
	if err != nil || len(ids) != 0 {
		t.Fatalf("expected no pending jobs, got %v err=%v", ids, err)
	}
}

func TestScriptGenerationJobFailureCanBeDiscarded(t *testing.T) {
	generator := &recordingScriptGenerator{err: errors.New("llm unavailable")}
	service, product, point := newScriptGenerationJobTestService(t, generator)
	userID := uuid.NewString()
	job := createScriptGenerationJobForTest(t, service, userID, product, point)

	if err := service.Process(context.Background(), job.ID); err == nil {
		t.Fatal("expected generation failure")
	}
	failed, err := service.GetForUser(context.Background(), job.ID, userID)
	if err != nil || failed.Status != ScriptGenerationJobStatusFailed || failed.ErrorMessage == "" {
		t.Fatalf("unexpected failed job %#v err=%v", failed, err)
	}
	discarded, err := service.ResolveForUser(context.Background(), job.ID, userID, ScriptGenerationJobStatusDiscarded)
	if err != nil || discarded.Status != ScriptGenerationJobStatusDiscarded {
		t.Fatalf("discard failed job: %#v err=%v", discarded, err)
	}
}
