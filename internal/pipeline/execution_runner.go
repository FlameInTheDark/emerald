package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/FlameInTheDark/emerald/internal/db/models"
)

type FlowSemanticValidator interface {
	ValidateFlowData(ctx context.Context, flowData FlowData, allowUnavailablePlugins bool) error
}

type SecretTemplateValueProvider interface {
	TemplateValues(ctx context.Context) (map[string]string, error)
}

type ExecutionStore interface {
	Create(ctx context.Context, e *models.Execution) error
	UpdateStatus(ctx context.Context, id, status string, completedAt *time.Time, errMsg *string) error
	CreateNodeExecution(ctx context.Context, ne *models.NodeExecution) error
	UpdateNodeExecution(ctx context.Context, id string, status string, output json.RawMessage, errMsg *string, completedAt *time.Time) error
}

type ExecutionBroadcaster interface {
	Broadcast(channel string, data any)
}

type ActiveExecutionInfo struct {
	ExecutionID          string     `json:"execution_id"`
	PipelineID           string     `json:"pipeline_id"`
	TriggerType          string     `json:"trigger_type"`
	Status               string     `json:"status"`
	StartedAt            time.Time  `json:"started_at"`
	CurrentNodeID        string     `json:"current_node_id,omitempty"`
	CurrentNodeType      string     `json:"current_node_type,omitempty"`
	CurrentNodeStartedAt *time.Time `json:"current_node_started_at,omitempty"`
}

type ExecutionRunResult struct {
	ExecutionID  string
	PipelineID   string
	TriggerType  string
	Status       string
	StartedAt    time.Time
	CompletedAt  time.Time
	Duration     time.Duration
	NodesRun     int
	Returned     bool
	ReturnValue  any
	Error        error
	ErrorMessage string
}

type ExecutionRunner struct {
	store       ExecutionStore
	engine      *Engine
	broadcaster ExecutionBroadcaster
	active      *activeExecutionTracker
	validator   FlowSemanticValidator
	secrets     SecretTemplateValueProvider
}

type ExecutionRunnerOption func(*ExecutionRunner)

func WithFlowSemanticValidator(validator FlowSemanticValidator) ExecutionRunnerOption {
	return func(r *ExecutionRunner) {
		r.validator = validator
	}
}

func WithSecretTemplateValueProvider(provider SecretTemplateValueProvider) ExecutionRunnerOption {
	return func(r *ExecutionRunner) {
		r.secrets = provider
	}
}

func NewExecutionRunner(store ExecutionStore, engine *Engine, broadcaster ExecutionBroadcaster, opts ...ExecutionRunnerOption) *ExecutionRunner {
	runner := &ExecutionRunner{
		store:       store,
		engine:      engine,
		broadcaster: broadcaster,
		active:      newActiveExecutionTracker(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(runner)
		}
	}
	return runner
}

func (r *ExecutionRunner) Run(
	ctx context.Context,
	pipelineID string,
	flowData FlowData,
	triggerType string,
	executionContext map[string]any,
) (*ExecutionRunResult, error) {
	if r.store == nil {
		return nil, fmt.Errorf("execution store is not configured")
	}
	if r.engine == nil {
		return nil, fmt.Errorf("execution engine is not configured")
	}
	if r.validator != nil {
		if err := r.validator.ValidateFlowData(ctx, flowData, false); err != nil {
			return nil, err
		}
	}

	persistedContext := copyExecutionContext(executionContext)
	runtimeContext := copyExecutionContext(executionContext)
	delete(persistedContext, "secret")
	if r.secrets != nil {
		secretValues, err := r.secrets.TemplateValues(ctx)
		if err != nil {
			return nil, fmt.Errorf("load secret template values: %w", err)
		}
		runtimeContext["secret"] = secretValues
	}

	execution := &models.Execution{
		PipelineID:  pipelineID,
		TriggerType: triggerType,
		Status:      "running",
	}

	if len(persistedContext) > 0 {
		if payload, err := json.Marshal(persistedContext); err == nil {
			serialized := string(payload)
			execution.Context = &serialized
		}
	}

	if err := r.store.Create(ctx, execution); err != nil {
		return nil, fmt.Errorf("create execution record: %w", err)
	}

	runCtx, cleanup := r.active.start(ctx, execution.ID, pipelineID, triggerType, execution.StartedAt)
	defer cleanup()
	storeCtx := context.WithoutCancel(runCtx)

	nodeExecutionIDs := make(map[string]string)
	channel := executionChannel(pipelineID)

	r.broadcast(channel, map[string]any{
		"type":         "execution_started",
		"pipeline":     pipelineID,
		"execution":    execution.ID,
		"trigger_type": triggerType,
		"status":       "running",
		"started_at":   execution.StartedAt,
	})

	observer := &ExecutionObserver{
		OnNodeStarted: func(start NodeStart) {
			r.active.updateNode(execution.ID, start)

			ne := nodeExecutionFromStart(execution.ID, start)
			if err := r.store.CreateNodeExecution(storeCtx, &ne); err == nil {
				nodeExecutionIDs[start.NodeID] = ne.ID
			}

			r.broadcast(channel, map[string]any{
				"type":       "node_started",
				"pipeline":   pipelineID,
				"execution":  execution.ID,
				"node_id":    ne.NodeID,
				"node_type":  ne.NodeType,
				"status":     ne.Status,
				"started_at": ne.StartedAt,
			})
		},
		OnNodeCompleted: func(run NodeRun) {
			ne := nodeExecutionFromRun(execution.ID, run)

			if nodeExecutionID, ok := nodeExecutionIDs[run.NodeID]; ok {
				_ = r.store.UpdateNodeExecution(storeCtx, nodeExecutionID, ne.Status, run.Result.Output, ne.Error, ne.CompletedAt)
			} else {
				_ = r.store.CreateNodeExecution(storeCtx, &ne)
			}

			r.active.clearNode(execution.ID, run.NodeID)
			r.broadcast(channel, map[string]any{
				"type":         "node_completed",
				"pipeline":     pipelineID,
				"execution":    execution.ID,
				"node_id":      ne.NodeID,
				"node_type":    ne.NodeType,
				"status":       ne.Status,
				"error":        getOptionalString(ne.Error),
				"output":       getOptionalString(ne.Output),
				"started_at":   ne.StartedAt,
				"completed_at": ne.CompletedAt,
			})
		},
	}

	state, execErr := r.engine.ExecuteWithInput(WithPipelineCall(runCtx, pipelineID), flowData, triggerType, runtimeContext, observer)
	completedAt := time.Now()

	status := "completed"
	var errMsg *string
	if execErr != nil {
		if isCancellationError(execErr) {
			status = "cancelled"
		} else {
			status = "failed"
		}
		message := execErr.Error()
		errMsg = &message
	}

	if err := r.store.UpdateStatus(storeCtx, execution.ID, status, &completedAt, errMsg); err != nil {
		return nil, fmt.Errorf("update execution status: %w", err)
	}

	r.broadcast(channel, map[string]any{
		"type":         "execution_completed",
		"pipeline":     pipelineID,
		"execution":    execution.ID,
		"status":       status,
		"error":        getOptionalString(errMsg),
		"completed_at": completedAt,
	})

	result := &ExecutionRunResult{
		ExecutionID: execution.ID,
		PipelineID:  pipelineID,
		TriggerType: triggerType,
		Status:      status,
		StartedAt:   execution.StartedAt,
		CompletedAt: completedAt,
		Duration:    completedAt.Sub(execution.StartedAt),
	}
	if state != nil {
		result.NodesRun = len(state.NodeRuns)
		if state.Returned {
			result.Returned = true
			result.ReturnValue = state.ReturnValue
		}
	}
	if execErr != nil {
		result.Error = execErr
		result.ErrorMessage = execErr.Error()
	}

	return result, nil
}

func (r *ExecutionRunner) ActiveByPipeline(pipelineID string) []ActiveExecutionInfo {
	if r == nil || r.active == nil {
		return []ActiveExecutionInfo{}
	}

	return r.active.listByPipeline(pipelineID)
}

func (r *ExecutionRunner) Cancel(executionID string) (ActiveExecutionInfo, bool) {
	if r == nil || r.active == nil {
		return ActiveExecutionInfo{}, false
	}

	info, ok := r.active.cancel(executionID)
	if !ok {
		return ActiveExecutionInfo{}, false
	}

	r.broadcast(executionChannel(info.PipelineID), map[string]any{
		"type":         "execution_cancelling",
		"pipeline":     info.PipelineID,
		"execution":    info.ExecutionID,
		"trigger_type": info.TriggerType,
		"status":       info.Status,
		"started_at":   info.StartedAt,
	})

	return info, true
}

func (r *ExecutionRunner) broadcast(channel string, payload map[string]any) {
	if r == nil || r.broadcaster == nil {
		return
	}

	r.broadcaster.Broadcast(channel, payload)
}

type trackedExecution struct {
	info   ActiveExecutionInfo
	cancel context.CancelFunc
}

type activeExecutionTracker struct {
	mu         sync.RWMutex
	executions map[string]*trackedExecution
}

func newActiveExecutionTracker() *activeExecutionTracker {
	return &activeExecutionTracker{
		executions: make(map[string]*trackedExecution),
	}
}

func (t *activeExecutionTracker) start(
	parent context.Context,
	executionID string,
	pipelineID string,
	triggerType string,
	startedAt time.Time,
) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)

	t.mu.Lock()
	t.executions[executionID] = &trackedExecution{
		info: ActiveExecutionInfo{
			ExecutionID: executionID,
			PipelineID:  pipelineID,
			TriggerType: triggerType,
			Status:      "running",
			StartedAt:   startedAt,
		},
		cancel: cancel,
	}
	t.mu.Unlock()

	return ctx, func() {
		cancel()

		t.mu.Lock()
		delete(t.executions, executionID)
		t.mu.Unlock()
	}
}

func (t *activeExecutionTracker) updateNode(executionID string, start NodeStart) {
	t.mu.Lock()
	defer t.mu.Unlock()

	execution := t.executions[executionID]
	if execution == nil {
		return
	}

	startedAt := start.StartedAt
	execution.info.CurrentNodeID = start.NodeID
	execution.info.CurrentNodeType = start.NodeType
	execution.info.CurrentNodeStartedAt = &startedAt
}

func (t *activeExecutionTracker) clearNode(executionID string, nodeID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	execution := t.executions[executionID]
	if execution == nil || execution.info.CurrentNodeID != nodeID {
		return
	}

	execution.info.CurrentNodeID = ""
	execution.info.CurrentNodeType = ""
	execution.info.CurrentNodeStartedAt = nil
}

func (t *activeExecutionTracker) listByPipeline(pipelineID string) []ActiveExecutionInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]ActiveExecutionInfo, 0)
	for _, execution := range t.executions {
		if execution == nil {
			continue
		}
		if pipelineID != "" && execution.info.PipelineID != pipelineID {
			continue
		}
		result = append(result, execution.info)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].StartedAt.After(result[j].StartedAt)
	})

	return result
}

func (t *activeExecutionTracker) cancel(executionID string) (ActiveExecutionInfo, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	execution := t.executions[executionID]
	if execution == nil {
		return ActiveExecutionInfo{}, false
	}

	if execution.info.Status != "cancelling" {
		execution.info.Status = "cancelling"
		execution.cancel()
	}

	return execution.info, true
}

func nodeExecutionFromStart(executionID string, start NodeStart) models.NodeExecution {
	nodeType := start.NodeType
	if nodeType == "" {
		nodeType = "unknown"
	}

	var inputStr *string
	if len(start.Input) > 0 {
		if data, err := json.Marshal(start.Input); err == nil {
			str := string(data)
			inputStr = &str
		}
	}

	startedAt := start.StartedAt

	return models.NodeExecution{
		ExecutionID: executionID,
		NodeID:      start.NodeID,
		NodeType:    nodeType,
		Status:      "running",
		Input:       inputStr,
		StartedAt:   &startedAt,
	}
}

func nodeExecutionFromRun(executionID string, run NodeRun) models.NodeExecution {
	nodeType := run.NodeType
	if nodeType == "" {
		nodeType = "unknown"
	}

	var inputStr *string
	if len(run.Input) > 0 {
		if data, err := json.Marshal(run.Input); err == nil {
			str := string(data)
			inputStr = &str
		}
	}

	var outputStr *string
	if len(run.Result.Output) > 0 {
		str := string(run.Result.Output)
		outputStr = &str
	}

	status := "completed"
	var errMsg *string
	if run.Result.Error != nil {
		if isCancellationError(run.Result.Error) {
			status = "cancelled"
		} else {
			status = "failed"
		}
		e := run.Result.Error.Error()
		errMsg = &e
	}

	startedAt := run.StartedAt
	completedAt := run.CompletedAt

	return models.NodeExecution{
		ExecutionID: executionID,
		NodeID:      run.NodeID,
		NodeType:    nodeType,
		Status:      status,
		Input:       inputStr,
		Output:      outputStr,
		Error:       errMsg,
		StartedAt:   &startedAt,
		CompletedAt: &completedAt,
	}
}

func getOptionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func executionChannel(pipelineID string) string {
	return "pipeline-" + pipelineID
}

func isCancellationError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
