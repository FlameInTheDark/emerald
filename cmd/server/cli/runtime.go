package cli

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	hplugin "github.com/hashicorp/go-plugin"

	"github.com/FlameInTheDark/emerald/internal/channels"
	"github.com/FlameInTheDark/emerald/internal/config"
	"github.com/FlameInTheDark/emerald/internal/crypto"
	"github.com/FlameInTheDark/emerald/internal/db"
	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/db/query"
	"github.com/FlameInTheDark/emerald/internal/node"
	"github.com/FlameInTheDark/emerald/internal/node/trigger"
	"github.com/FlameInTheDark/emerald/internal/nodedefs"
	"github.com/FlameInTheDark/emerald/internal/pipeline"
	"github.com/FlameInTheDark/emerald/internal/pipelineops"
	"github.com/FlameInTheDark/emerald/internal/plugins"
	"github.com/FlameInTheDark/emerald/internal/scheduler"
	"github.com/FlameInTheDark/emerald/internal/shellcmd"
	"github.com/FlameInTheDark/emerald/internal/skills"
	triggerservice "github.com/FlameInTheDark/emerald/internal/triggers"
	"github.com/FlameInTheDark/emerald/internal/ws"
	"github.com/FlameInTheDark/emerald/pkg/pluginapi"
)

type cliRuntimeOptions struct {
	migrate bool
}

type channelDispatchController struct {
	mu      sync.RWMutex
	handler channels.MessageDispatch
}

func (c *channelDispatchController) Dispatch(ctx context.Context, event trigger.ChannelEvent) error {
	c.mu.RLock()
	handler := c.handler
	c.mu.RUnlock()
	if handler == nil {
		return nil
	}
	return handler(ctx, event)
}

func (c *channelDispatchController) Set(handler channels.MessageDispatch) {
	c.mu.Lock()
	c.handler = handler
	c.mu.Unlock()
}

type runtimeBundle struct {
	Config        *config.Config
	Database      *db.DB
	EncryptionKey string
	Encryptor     *crypto.Encryptor
	WorkingDir    string
	SkillsDir     string
	PluginsDir    string

	AppConfigStore         *query.AppConfigStore
	AuditLogStore          *query.AuditLogStore
	ClusterStore           *query.ClusterStore
	KubernetesClusterStore *query.KubernetesClusterStore
	LLMProviderStore       *query.LLMProviderStore
	ChannelStore           *query.ChannelStore
	ChannelContactStore    *query.ChannelContactStore
	UserStore              *query.UserStore
	UserSessionStore       *query.UserSessionStore
	SecretStore            *query.SecretStore
	PipelineStore          *query.PipelineStore
	ExecutionStore         *query.ExecutionStore

	PluginManager         *plugins.Manager
	NodeDefinitionService *nodedefs.Service
	SkillStore            skills.ManagedReader
	ShellRunner           shellcmd.Runner
	ChannelService        *channels.Service
	WSHub                 *ws.Hub
	Registry              *node.Registry
	Engine                *pipeline.Engine
	ExecutionRunner       *pipeline.ExecutionRunner
	PipelineInvoker       *pipeline.Invoker
	CronScheduler         *scheduler.Scheduler
	TriggerService        *triggerservice.Service
	PipelineManager       *pipelineops.Service

	dispatchController   *channelDispatchController
	pluginTriggerService *plugins.TriggerRuntimeService

	wsHubStarted          bool
	skillStoreStarted     bool
	channelServiceStarted bool
	cronSchedulerStarted  bool
	cleanupOnce           sync.Once
}

type runtimePipelineRunner struct {
	runtime *runtimeBundle
}

func (r runtimePipelineRunner) Run(ctx context.Context, pipelineID string, input map[string]any) (*pipeline.RunResult, error) {
	if r.runtime == nil || r.runtime.PipelineInvoker == nil {
		return nil, fmt.Errorf("pipeline runner is not configured")
	}
	return r.runtime.PipelineInvoker.Run(ctx, pipelineID, input)
}

type runtimePipelineMutationManager struct {
	runtime *runtimeBundle
}

func (m runtimePipelineMutationManager) Create(ctx context.Context, pipelineModel *models.Pipeline) error {
	if m.runtime == nil || m.runtime.PipelineManager == nil {
		return fmt.Errorf("pipeline manager is not configured")
	}
	return m.runtime.PipelineManager.Create(ctx, pipelineModel)
}

func (m runtimePipelineMutationManager) Update(ctx context.Context, pipelineModel *models.Pipeline) error {
	if m.runtime == nil || m.runtime.PipelineManager == nil {
		return fmt.Errorf("pipeline manager is not configured")
	}
	return m.runtime.PipelineManager.Update(ctx, pipelineModel)
}

func (m runtimePipelineMutationManager) Delete(ctx context.Context, ref pipelineops.Reference) (*models.Pipeline, error) {
	if m.runtime == nil || m.runtime.PipelineManager == nil {
		return nil, fmt.Errorf("pipeline manager is not configured")
	}
	return m.runtime.PipelineManager.Delete(ctx, ref)
}

func (m runtimePipelineMutationManager) Resolve(ctx context.Context, ref pipelineops.Reference) (*models.Pipeline, error) {
	if m.runtime == nil || m.runtime.PipelineManager == nil {
		return nil, fmt.Errorf("pipeline manager is not configured")
	}
	return m.runtime.PipelineManager.Resolve(ctx, ref)
}

func newCLIRuntime(ctx context.Context, opts cliRuntimeOptions) (*runtimeBundle, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	database, err := db.New(cfg.Database.Path)
	if err != nil {
		return nil, fmt.Errorf("initialize database: %w", err)
	}

	cleanupDatabase := true
	defer func() {
		if cleanupDatabase {
			_ = database.Close()
		}
	}()

	if opts.migrate {
		if err := db.Migrate(database); err != nil {
			return nil, fmt.Errorf("run migrations: %w", err)
		}
	}

	appConfigStore := query.NewAppConfigStore(database.DB)
	encryptionKey, err := resolveEncryptionKey(ctx, appConfigStore, cfg.Security)
	if err != nil {
		return nil, fmt.Errorf("initialize encryption key: %w", err)
	}

	encryptor, err := crypto.NewEncryptor(encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("create encryptor: %w", err)
	}

	workingDir, err := os.Getwd()
	if err != nil {
		log.Printf("failed to resolve working directory: %v", err)
		workingDir = "."
	}

	executablePath, err := os.Executable()
	if err != nil {
		log.Printf("failed to resolve executable path: %v", err)
		executablePath = ""
	}

	resolveSkillsDir := func() (string, error) {
		hints := make([]string, 0, 3)
		if currentDir, err := os.Getwd(); err == nil && strings.TrimSpace(currentDir) != "" {
			hints = append(hints, currentDir)
		}
		if strings.TrimSpace(workingDir) != "" {
			hints = append(hints, workingDir)
		}
		if strings.TrimSpace(executablePath) != "" {
			hints = append(hints, executablePath)
		}
		return skills.ResolveDirectory(os.Getenv("EMERALD_SKILLS_DIR"), hints...)
	}

	skillsDir, err := resolveSkillsDir()
	if err != nil {
		log.Printf("failed to resolve skills directory: %v", err)
		skillsDir = filepath.Join(workingDir, ".agents", "skills")
	}

	var seededSkillDirs sync.Map
	ensureSkillDefaults := func(dir string) {
		trimmed := strings.TrimSpace(dir)
		if trimmed == "" {
			return
		}
		if _, loaded := seededSkillDirs.LoadOrStore(trimmed, struct{}{}); loaded {
			return
		}
		if err := skills.EnsureBundledDefaults(trimmed); err != nil {
			log.Printf("failed to seed bundled skills: %v", err)
			seededSkillDirs.Delete(trimmed)
		}
	}
	ensureSkillDefaults(skillsDir)
	resolveManagedSkillsDir := func() (string, error) {
		dir, err := resolveSkillsDir()
		if err != nil {
			return "", err
		}
		ensureSkillDefaults(dir)
		return dir, nil
	}
	log.Printf("loading local skills from %s", skillsDir)

	pluginsDir, err := plugins.ResolveDirectory(os.Getenv("EMERALD_PLUGINS_DIR"), workingDir, executablePath)
	if err != nil {
		log.Printf("failed to resolve plugins directory: %v", err)
		pluginsDir = filepath.Join(workingDir, ".agents", "plugins")
	}
	log.Printf("loading local plugins from %s", pluginsDir)

	pluginManager := plugins.NewManager(pluginsDir, cfg.Security.AllowPlugins)
	if err := pluginManager.Refresh(ctx); err != nil {
		log.Printf("failed to load plugins: %v", err)
	}

	runtime := &runtimeBundle{
		Config:                 cfg,
		Database:               database,
		EncryptionKey:          encryptionKey,
		Encryptor:              encryptor,
		WorkingDir:             workingDir,
		SkillsDir:              skillsDir,
		PluginsDir:             pluginsDir,
		AppConfigStore:         appConfigStore,
		AuditLogStore:          query.NewAuditLogStore(database.DB),
		ClusterStore:           query.NewClusterStore(database.DB, encryptor),
		KubernetesClusterStore: query.NewKubernetesClusterStore(database.DB, encryptor),
		LLMProviderStore:       query.NewLLMProviderStore(database.DB, encryptor),
		ChannelStore:           query.NewChannelStore(database.DB, encryptor),
		ChannelContactStore:    query.NewChannelContactStore(database.DB),
		UserStore:              query.NewUserStore(database.DB, encryptor),
		UserSessionStore:       query.NewUserSessionStore(database.DB),
		SecretStore:            query.NewSecretStore(database.DB, encryptor),
		PipelineStore:          query.NewPipelineStore(database.DB),
		ExecutionStore:         query.NewExecutionStore(database.DB),
		PluginManager:          pluginManager,
		NodeDefinitionService:  nodedefs.NewService(pluginManager),
		SkillStore:             skills.NewResolvingStore(resolveManagedSkillsDir, 2*time.Second),
		ShellRunner:            shellcmd.NewRunner(workingDir, cfg.Security.AllowAbsoluteToolPaths),
		WSHub:                  ws.NewHub(),
		dispatchController:     &channelDispatchController{},
	}

	runtime.pluginTriggerService = plugins.NewTriggerRuntimeService(
		runtime.PluginManager,
		func(ctx context.Context, subscription pluginapi.TriggerSubscription, event *pluginapi.TriggerEvent) error {
			if runtime.TriggerService == nil {
				return nil
			}
			return runtime.TriggerService.HandlePluginEvent(ctx, subscription, event)
		},
	)

	runtime.ChannelService = channels.NewService(
		runtime.ChannelStore,
		runtime.ChannelContactStore,
		func(ctx context.Context, event trigger.ChannelEvent) error {
			return runtime.dispatchController.Dispatch(ctx, event)
		},
	)

	runtime.Registry = buildNodeRegistry(registryDependencies{
		ClusterStore:           runtime.ClusterStore,
		KubernetesClusterStore: runtime.KubernetesClusterStore,
		LLMProviderStore:       runtime.LLMProviderStore,
		ChannelStore:           runtime.ChannelStore,
		ChannelContactStore:    runtime.ChannelContactStore,
		ChannelService:         runtime.ChannelService,
		PipelineStore:          runtime.PipelineStore,
		PipelineRunner:         runtimePipelineRunner{runtime: runtime},
		PipelineManager:        runtimePipelineMutationManager{runtime: runtime},
		SkillStore:             runtime.SkillStore,
		ShellRunner:            runtime.ShellRunner,
		PluginManager:          runtime.PluginManager,
	})

	runtime.Engine = pipeline.NewEngine(runtime.Registry)
	runtime.ExecutionRunner = pipeline.NewExecutionRunner(
		runtime.ExecutionStore,
		runtime.Engine,
		runtime.WSHub,
		pipeline.WithFlowSemanticValidator(runtime.NodeDefinitionService),
		pipeline.WithSecretTemplateValueProvider(runtime.SecretStore),
	)
	runtime.PipelineInvoker = pipeline.NewInvoker(runtime.Database.DB, runtime.PipelineStore, runtime.Engine, runtime.ExecutionRunner)
	runtime.CronScheduler = scheduler.New(runtime.Database.DB, func(ctx context.Context, pipelineID string, rootNodeID string) error {
		flowData, err := scheduler.LoadFlowData(runtime.Database.DB, pipelineID)
		if err != nil {
			return err
		}

		result, err := runtime.ExecutionRunner.Run(
			ctx,
			pipelineID,
			*flowData,
			pipeline.TriggerSelectionFromNodeIDs("cron", []string{rootNodeID}),
			nil,
		)
		if err != nil {
			return err
		}
		if result.Status == "failed" && result.Error != nil {
			return result.Error
		}
		return nil
	})
	runtime.TriggerService = triggerservice.NewService(
		runtime.PipelineStore,
		runtime.CronScheduler,
		runtime.ExecutionRunner,
		runtime.pluginTriggerService,
	)
	runtime.PipelineManager = pipelineops.NewService(runtime.PipelineStore, runtime.TriggerService, runtime.NodeDefinitionService)
	runtime.PipelineManager.SetActivationValidator(runtime.TriggerService)

	cleanupDatabase = false
	return runtime, nil
}

func (r *runtimeBundle) startExecutionServices(ctx context.Context) error {
	if r == nil {
		return fmt.Errorf("runtime is required")
	}

	if !r.wsHubStarted {
		r.wsHubStarted = true
		go r.WSHub.Run()
	}

	if !r.skillStoreStarted {
		if err := r.SkillStore.Start(ctx); err != nil {
			return fmt.Errorf("start skill store: %w", err)
		}
		r.skillStoreStarted = true
	}

	if !r.channelServiceStarted {
		if err := r.ChannelService.Start(); err != nil {
			return fmt.Errorf("start channel service: %w", err)
		}
		r.channelServiceStarted = true
	}

	return nil
}

func (r *runtimeBundle) startServerServices(ctx context.Context) error {
	if r == nil {
		return fmt.Errorf("runtime is required")
	}

	r.dispatchController.Set(r.handleChannelEvent)

	if err := r.startExecutionServices(ctx); err != nil {
		return err
	}

	if !r.cronSchedulerStarted {
		r.CronScheduler.Start()
		r.cronSchedulerStarted = true
	}

	if err := r.TriggerService.Reload(ctx); err != nil {
		return fmt.Errorf("reload trigger runtimes: %w", err)
	}

	return nil
}

func (r *runtimeBundle) handleChannelEvent(ctx context.Context, event trigger.ChannelEvent) error {
	if r == nil || r.ExecutionRunner == nil || r.PipelineStore == nil {
		return nil
	}

	eventCtx := trigger.WithChannelEvent(ctx, event)
	activePipelines, err := r.PipelineStore.ListActive(eventCtx)
	if err != nil {
		return err
	}

	executionContext := map[string]any{
		"channel_id":       event.ChannelID,
		"channel_name":     event.ChannelName,
		"channel_type":     event.ChannelType,
		"contact_id":       event.ContactID,
		"external_user_id": event.ExternalUserID,
		"external_chat_id": event.ExternalChatID,
		"text":             event.Text,
		"message":          event.Message,
	}

	for _, pipelineModel := range activePipelines {
		flowData, err := pipeline.ParseFlowData(pipelineModel.Nodes, pipelineModel.Edges)
		if err != nil {
			log.Printf("failed to parse channel pipeline %s: %v", pipelineModel.ID, err)
			continue
		}

		selection := pipeline.ResolveTriggerSelection(eventCtx, *flowData, pipeline.TriggerSelection{TriggerType: "channel"})
		if len(selection.RootNodeIDs) == 0 {
			continue
		}

		result, err := r.ExecutionRunner.Run(eventCtx, pipelineModel.ID, *flowData, selection, executionContext)
		if err != nil {
			log.Printf("channel pipeline %s execution failed: %v", pipelineModel.ID, err)
			continue
		}
		if result.Status == "failed" && result.Error != nil {
			log.Printf("channel pipeline %s execution failed: %v", pipelineModel.ID, result.Error)
		}
	}

	return nil
}

func (r *runtimeBundle) Close() error {
	if r == nil {
		return nil
	}

	var closeErr error
	r.cleanupOnce.Do(func() {
		if r.channelServiceStarted {
			r.ChannelService.Stop()
		}
		if r.cronSchedulerStarted {
			r.CronScheduler.Stop()
		}
		if r.TriggerService != nil {
			r.TriggerService.Stop()
		} else if r.pluginTriggerService != nil {
			r.pluginTriggerService.Stop()
		}
		if r.skillStoreStarted {
			r.SkillStore.Stop()
		}
		if r.PluginManager != nil {
			r.PluginManager.Stop()
		}
		hplugin.CleanupClients()
		if r.Database != nil {
			closeErr = r.Database.Close()
		}
	})

	return closeErr
}

func resolveEncryptionKey(ctx context.Context, store *query.AppConfigStore, security config.SecurityConfig) (string, error) {
	if trimmed := strings.TrimSpace(security.EncryptionKey); trimmed != "" {
		return trimmed, nil
	}

	if !security.AllowDBStoredKey {
		return "", fmt.Errorf("EMERALD_ENCRYPTION_KEY is required")
	}
	if store == nil {
		return "", fmt.Errorf("app config store is not configured")
	}

	key, ok, err := store.GetEncryptionKey(ctx)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("no legacy encryption key is stored in app_configs")
	}

	log.Printf("warning: using legacy database-stored encryption key because EMERALD_ALLOW_DB_STORED_KEY is enabled")
	return key, nil
}
