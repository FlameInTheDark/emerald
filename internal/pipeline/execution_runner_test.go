package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/node"
)

type blockingExecutor struct {
	started chan struct{}
}

func (e *blockingExecutor) Execute(ctx context.Context, _ json.RawMessage, _ map[string]any) (*node.NodeResult, error) {
	select {
	case e.started <- struct{}{}:
	default:
	}

	<-ctx.Done()
	return nil, ctx.Err()
}

func (e *blockingExecutor) Validate(_ json.RawMessage) error {
	return nil
}

type runnerFailingExecutor struct {
	err error
}

func (e *runnerFailingExecutor) Execute(context.Context, json.RawMessage, map[string]any) (*node.NodeResult, error) {
	return nil, e.err
}

func (e *runnerFailingExecutor) Validate(json.RawMessage) error {
	return nil
}

type runnerPassthroughExecutor struct{}

func (e *runnerPassthroughExecutor) Execute(_ context.Context, _ json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	data, _ := json.Marshal(input)
	return &node.NodeResult{Output: data}, nil
}

func (e *runnerPassthroughExecutor) Validate(json.RawMessage) error {
	return nil
}

type staticSecretProvider struct {
	values map[string]string
	err    error
}

func (p staticSecretProvider) TemplateValues(context.Context) (map[string]string, error) {
	if p.err != nil {
		return nil, p.err
	}

	result := make(map[string]string, len(p.values))
	for key, value := range p.values {
		result[key] = value
	}
	return result, nil
}

type testExecutionStore struct {
	mu             sync.Mutex
	executions     map[string]*models.Execution
	nodeExecutions map[string]*models.NodeExecution
}

func newTestExecutionStore() *testExecutionStore {
	return &testExecutionStore{
		executions:     make(map[string]*models.Execution),
		nodeExecutions: make(map[string]*models.NodeExecution),
	}
}

func (s *testExecutionStore) Create(_ context.Context, e *models.Execution) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	copyValue := *e
	copyValue.ID = "exec-1"
	e.ID = copyValue.ID
	if copyValue.StartedAt.IsZero() {
		copyValue.StartedAt = time.Now()
		e.StartedAt = copyValue.StartedAt
	}
	s.executions[e.ID] = &copyValue
	return nil
}

func (s *testExecutionStore) UpdateStatus(_ context.Context, id, status string, completedAt *time.Time, errMsg *string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	execution := s.executions[id]
	if execution == nil {
		return errors.New("execution not found")
	}

	execution.Status = status
	execution.CompletedAt = completedAt
	execution.Error = errMsg
	return nil
}

func (s *testExecutionStore) CreateNodeExecution(_ context.Context, ne *models.NodeExecution) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	copyValue := *ne
	copyValue.ID = "node-" + ne.NodeID
	ne.ID = copyValue.ID
	s.nodeExecutions[ne.ID] = &copyValue
	return nil
}

func (s *testExecutionStore) UpdateNodeExecution(_ context.Context, id string, status string, output json.RawMessage, errMsg *string, completedAt *time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	nodeExecution := s.nodeExecutions[id]
	if nodeExecution == nil {
		return errors.New("node execution not found")
	}

	nodeExecution.Status = status
	if len(output) > 0 {
		outputValue := string(output)
		nodeExecution.Output = &outputValue
	}
	nodeExecution.Error = errMsg
	nodeExecution.CompletedAt = completedAt
	return nil
}

type broadcastRecorder struct {
	mu       sync.Mutex
	messages []map[string]any
}

func (r *broadcastRecorder) Broadcast(_ string, data any) {
	payload, ok := data.(map[string]any)
	if !ok {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, payload)
}

func (r *broadcastRecorder) hasMessageType(messageType string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, message := range r.messages {
		if message["type"] == messageType {
			return true
		}
	}

	return false
}

func TestExecutionRunner_CancelTracksActiveExecution(t *testing.T) {
	registry := node.NewRegistry()
	executor := &blockingExecutor{started: make(chan struct{}, 1)}
	registry.Register("test:blocking", executor)

	engine := NewEngine(registry)
	store := newTestExecutionStore()
	broadcaster := &broadcastRecorder{}
	runner := NewExecutionRunner(store, engine, broadcaster)

	flowData := FlowData{
		Nodes: []FlowNode{
			{ID: "blocking", Type: "test:blocking"},
		},
	}

	resultCh := make(chan *ExecutionRunResult, 1)
	errCh := make(chan error, 1)

	go func() {
		result, err := runner.Run(context.Background(), "pipeline-1", flowData, "manual", nil)
		resultCh <- result
		errCh <- err
	}()

	select {
	case <-executor.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for node start")
	}

	deadline := time.Now().Add(2 * time.Second)
	var active []ActiveExecutionInfo
	for time.Now().Before(deadline) {
		active = runner.ActiveByPipeline("pipeline-1")
		if len(active) == 1 && active[0].CurrentNodeID == "blocking" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if len(active) != 1 {
		t.Fatalf("expected 1 active execution, got %d", len(active))
	}
	if active[0].CurrentNodeID != "blocking" {
		t.Fatalf("expected current node to be blocking, got %q", active[0].CurrentNodeID)
	}

	info, ok := runner.Cancel(active[0].ExecutionID)
	if !ok {
		t.Fatal("expected cancellation to succeed")
	}
	if info.Status != "cancelling" {
		t.Fatalf("expected status cancelling, got %q", info.Status)
	}

	var result *ExecutionRunResult
	select {
	case result = <-resultCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for run result")
	}

	if err := <-errCh; err != nil {
		t.Fatalf("runner returned unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected run result")
	}
	if result.Status != "cancelled" {
		t.Fatalf("expected cancelled status, got %q", result.Status)
	}
	if !errors.Is(result.Error, context.Canceled) {
		t.Fatalf("expected context canceled error, got %v", result.Error)
	}

	if active := runner.ActiveByPipeline("pipeline-1"); len(active) != 0 {
		t.Fatalf("expected no active executions after cancellation, got %d", len(active))
	}

	execution := store.executions[result.ExecutionID]
	if execution == nil {
		t.Fatal("expected execution to be stored")
	}
	if execution.Status != "cancelled" {
		t.Fatalf("expected stored execution status cancelled, got %q", execution.Status)
	}

	nodeExecution := store.nodeExecutions["node-blocking"]
	if nodeExecution == nil {
		t.Fatal("expected node execution to be stored")
	}
	if nodeExecution.Status != "cancelled" {
		t.Fatalf("expected node execution status cancelled, got %q", nodeExecution.Status)
	}

	if !broadcaster.hasMessageType("execution_started") {
		t.Fatal("expected execution_started broadcast")
	}
	if !broadcaster.hasMessageType("execution_cancelling") {
		t.Fatal("expected execution_cancelling broadcast")
	}
	if !broadcaster.hasMessageType("execution_completed") {
		t.Fatal("expected execution_completed broadcast")
	}
}

func TestExecutionRunner_ContinuedNodeErrorKeepsExecutionCompleted(t *testing.T) {
	registry := node.NewRegistry()
	registry.Register("test:fail", &runnerFailingExecutor{err: errors.New("boom")})
	registry.Register("test:next", &runnerPassthroughExecutor{})

	engine := NewEngine(registry)
	store := newTestExecutionStore()
	runner := NewExecutionRunner(store, engine, nil)

	flowData := FlowData{
		Nodes: []FlowNode{
			{
				ID:   "fail",
				Type: "test:fail",
				Data: json.RawMessage(`{"config":{"errorPolicy":"continue"}}`),
			},
			{ID: "next", Type: "test:next"},
		},
		Edges: []FlowEdge{
			{ID: "edge-1", Source: "fail", Target: "next"},
		},
	}

	result, err := runner.Run(context.Background(), "pipeline-1", flowData, "manual", map[string]any{"requestId": "req-1"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Status != "completed" {
		t.Fatalf("status = %q, want completed", result.Status)
	}

	execution := store.executions[result.ExecutionID]
	if execution == nil {
		t.Fatal("expected execution to be stored")
	}
	if execution.Status != "completed" {
		t.Fatalf("stored execution status = %q, want completed", execution.Status)
	}

	failedNode := store.nodeExecutions["node-fail"]
	if failedNode == nil {
		t.Fatal("expected failed node execution to be stored")
	}
	if failedNode.Status != "failed" {
		t.Fatalf("failed node status = %q, want failed", failedNode.Status)
	}

	nextNode := store.nodeExecutions["node-next"]
	if nextNode == nil {
		t.Fatal("expected downstream node execution to be stored")
	}
	if nextNode.Status != "completed" {
		t.Fatalf("downstream node status = %q, want completed", nextNode.Status)
	}
	if nextNode.Output == nil {
		t.Fatal("expected downstream output to be stored")
	}
	if !json.Valid([]byte(*nextNode.Output)) {
		t.Fatalf("downstream output should be valid JSON, got %q", *nextNode.Output)
	}

	var output map[string]any
	if err := json.Unmarshal([]byte(*nextNode.Output), &output); err != nil {
		t.Fatalf("unmarshal downstream output: %v", err)
	}

	if got := output["error"]; got != "boom" {
		t.Fatalf("output.error = %#v, want boom", got)
	}
	if got := output["errorNodeId"]; got != "fail" {
		t.Fatalf("output.errorNodeId = %#v, want fail", got)
	}
	if got := output["requestId"]; got != "req-1" {
		t.Fatalf("output.requestId = %#v, want req-1", got)
	}
}

func TestExecutionRunner_RuntimeSecretsOverrideReservedContextWithoutPersistingValues(t *testing.T) {
	t.Parallel()

	registry := node.NewRegistry()
	registry.Register("test:next", &runnerPassthroughExecutor{})

	engine := NewEngine(registry)
	store := newTestExecutionStore()
	runner := NewExecutionRunner(
		store,
		engine,
		nil,
		WithSecretTemplateValueProvider(staticSecretProvider{
			values: map[string]string{
				"db_password": "vault-secret",
			},
		}),
	)

	flowData := FlowData{
		Nodes: []FlowNode{
			{ID: "next", Type: "test:next"},
		},
	}

	result, err := runner.Run(context.Background(), "pipeline-1", flowData, "manual", map[string]any{
		"requestId": "req-1",
		"secret": map[string]any{
			"db_password": "user-supplied",
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	execution := store.executions[result.ExecutionID]
	if execution == nil {
		t.Fatal("expected execution to be stored")
	}
	if execution.Context == nil {
		t.Fatal("expected execution context to be stored")
	}
	if strings.Contains(*execution.Context, "vault-secret") {
		t.Fatalf("expected persisted execution context to omit injected secrets, got %s", *execution.Context)
	}
	if strings.Contains(*execution.Context, `"secret"`) {
		t.Fatalf("expected reserved secret key to be omitted from persisted execution context, got %s", *execution.Context)
	}

	nodeExecution := store.nodeExecutions["node-next"]
	if nodeExecution == nil || nodeExecution.Output == nil {
		t.Fatalf("expected node output to be stored, got %#v", nodeExecution)
	}

	var output map[string]any
	if err := json.Unmarshal([]byte(*nodeExecution.Output), &output); err != nil {
		t.Fatalf("unmarshal node output: %v", err)
	}

	secretPayload, ok := output["secret"].(map[string]any)
	if !ok {
		t.Fatalf("expected output.secret map, got %#v", output["secret"])
	}
	if got := secretPayload["db_password"]; got != "vault-secret" {
		t.Fatalf("output.secret.db_password = %#v, want vault-secret", got)
	}
	if got := output["requestId"]; got != "req-1" {
		t.Fatalf("output.requestId = %#v, want req-1", got)
	}
}
