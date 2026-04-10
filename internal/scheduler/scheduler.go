package scheduler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"

	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/pipeline"
)

type PipelineRunner func(ctx context.Context, pipelineID string) error

type Scheduler struct {
	cron           *cron.Cron
	db             *sql.DB
	pipelineRunner PipelineRunner
	entries        map[string]cron.EntryID
}

func New(db *sql.DB, runner PipelineRunner) *Scheduler {
	return &Scheduler{
		cron:           cron.New(cron.WithSeconds()),
		db:             db,
		pipelineRunner: runner,
		entries:        make(map[string]cron.EntryID),
	}
}

func (s *Scheduler) Start() {
	if err := s.Reload(context.Background()); err != nil {
		log.Printf("failed to load cron jobs: %v", err)
	}
	s.cron.Start()
	log.Println("cron scheduler started")
}

func (s *Scheduler) Stop() {
	s.clearJobs()
	s.cron.Stop()
	log.Println("cron scheduler stopped")
}

func (s *Scheduler) AddJob(jobKey, pipelineID, schedule string) error {
	sched, err := cron.ParseStandard(schedule)
	if err != nil {
		return fmt.Errorf("parse schedule: %w", err)
	}

	entryID := s.cron.Schedule(sched, cron.NewChain(cron.Recover(cron.DefaultLogger)).Then(cron.FuncJob(func() {
		ctx := context.Background()

		if s.pipelineRunner != nil {
			if err := s.pipelineRunner(ctx, pipelineID); err != nil {
				log.Printf("cron job %s: pipeline execution failed: %v", jobKey, err)
			}
		}
	})))

	s.entries[jobKey] = entryID

	return nil
}

func (s *Scheduler) RemoveJob(id cron.EntryID) {
	s.cron.Remove(id)
}

func (s *Scheduler) Entries() []cron.Entry {
	return s.cron.Entries()
}

func (s *Scheduler) Reload(ctx context.Context) error {
	s.clearJobs()

	rows, err := s.db.QueryContext(ctx, "SELECT id, nodes, edges FROM pipelines WHERE status = 'active'")
	if err != nil {
		return fmt.Errorf("query active pipelines for cron sync: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var pipelineID, nodesJSON, edgesJSON string
		if err := rows.Scan(&pipelineID, &nodesJSON, &edgesJSON); err != nil {
			return fmt.Errorf("scan active pipeline for cron sync: %w", err)
		}

		flowData, err := pipeline.ParseFlowData(nodesJSON, edgesJSON)
		if err != nil {
			log.Printf("skip cron sync for pipeline %s: %v", pipelineID, err)
			continue
		}

		for _, flowNode := range flowData.Nodes {
			nodeType, configData := pipelineNodeTypeAndConfig(flowNode)
			if nodeType != "trigger:cron" {
				continue
			}

			var cfg struct {
				Schedule string `json:"schedule"`
			}
			if err := json.Unmarshal(configData, &cfg); err != nil {
				log.Printf("skip cron node %s in pipeline %s: %v", flowNode.ID, pipelineID, err)
				continue
			}
			if strings.TrimSpace(cfg.Schedule) == "" {
				continue
			}

			if err := s.AddJob(pipelineID+":"+flowNode.ID, pipelineID, cfg.Schedule); err != nil {
				log.Printf("failed to add cron job for pipeline %s node %s: %v", pipelineID, flowNode.ID, err)
			}
		}
	}

	return rows.Err()
}

func (s *Scheduler) ActiveJobCount() int {
	return len(s.entries)
}

func (s *Scheduler) clearJobs() {
	for key, entryID := range s.entries {
		s.cron.Remove(entryID)
		delete(s.entries, key)
	}
}

func ExecutePipeline(ctx context.Context, db *sql.DB, pipelineID string, engine *pipeline.Engine, flowData pipeline.FlowData) error {
	return ExecutePipelineWithTrigger(ctx, db, pipelineID, engine, flowData, "cron", nil)
}

func ExecutePipelineWithTrigger(
	ctx context.Context,
	db *sql.DB,
	pipelineID string,
	engine *pipeline.Engine,
	flowData pipeline.FlowData,
	triggerType string,
	executionContext map[string]any,
) error {
	execution := &models.Execution{
		ID:          uuid.New().String(),
		PipelineID:  pipelineID,
		TriggerType: triggerType,
		Status:      "running",
		StartedAt:   time.Now(),
	}

	var contextJSON *string
	if executionContext != nil {
		if data, err := json.Marshal(executionContext); err == nil {
			str := string(data)
			contextJSON = &str
		}
	}
	execution.Context = contextJSON

	query := `INSERT INTO executions (id, pipeline_id, trigger_type, status, started_at, context) VALUES (?, ?, ?, ?, ?, ?)`
	_, err := db.ExecContext(ctx, query, execution.ID, execution.PipelineID, execution.TriggerType, execution.Status, execution.StartedAt, execution.Context)
	if err != nil {
		return fmt.Errorf("create execution record: %w", err)
	}

	state, err := engine.ExecuteWithInput(pipeline.WithPipelineCall(ctx, pipelineID), flowData, triggerType, executionContext)

	completedAt := time.Now()
	if err != nil {
		errMsg := err.Error()
		_, _ = db.ExecContext(ctx,
			"UPDATE executions SET status = ?, completed_at = ?, error = ? WHERE id = ?",
			"failed", completedAt, errMsg, execution.ID,
		)
		return fmt.Errorf("execute pipeline: %w", err)
	}

	_, _ = db.ExecContext(ctx,
		"UPDATE executions SET status = ?, completed_at = ? WHERE id = ?",
		"completed", completedAt, execution.ID,
	)

	for _, run := range state.NodeRuns {
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

		ne := &models.NodeExecution{
			ID:          uuid.New().String(),
			ExecutionID: execution.ID,
			NodeID:      run.NodeID,
			NodeType:    run.NodeType,
			Status:      "completed",
			Input:       inputStr,
			Output:      outputStr,
			StartedAt:   &run.StartedAt,
			CompletedAt: &run.CompletedAt,
		}
		if run.Result.Error != nil {
			ne.Status = "failed"
			errStr := run.Result.Error.Error()
			ne.Error = &errStr
		}

		_, _ = db.ExecContext(ctx,
			`INSERT INTO node_executions (id, execution_id, node_id, node_type, status, input, output, error, started_at, completed_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			ne.ID, ne.ExecutionID, ne.NodeID, ne.NodeType, ne.Status,
			ne.Input, ne.Output, ne.Error, ne.StartedAt, ne.CompletedAt,
		)
	}

	return nil
}

func LoadFlowData(db *sql.DB, pipelineID string) (*pipeline.FlowData, error) {
	var nodesJSON, edgesJSON string
	err := db.QueryRow("SELECT nodes, edges FROM pipelines WHERE id = ?", pipelineID).Scan(&nodesJSON, &edgesJSON)
	if err != nil {
		return nil, fmt.Errorf("query pipeline: %w", err)
	}

	return pipeline.ParseFlowData(nodesJSON, edgesJSON)
}

func pipelineNodeTypeAndConfig(flowNode pipeline.FlowNode) (string, json.RawMessage) {
	nodeType := flowNode.Type
	config := flowNode.Data

	if len(flowNode.Data) == 0 {
		return nodeType, config
	}

	var nodeData map[string]json.RawMessage
	if err := json.Unmarshal(flowNode.Data, &nodeData); err != nil {
		return nodeType, config
	}

	if rawType, ok := nodeData["type"]; ok {
		var typeString string
		if err := json.Unmarshal(rawType, &typeString); err == nil && typeString != "" {
			nodeType = typeString
		}
	}
	if rawConfig, ok := nodeData["config"]; ok {
		config = rawConfig
	}

	return nodeType, config
}
