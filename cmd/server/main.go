package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/FlameInTheDark/emerald/internal/api"
	"github.com/FlameInTheDark/emerald/internal/auth"
	"github.com/FlameInTheDark/emerald/internal/channels"
	"github.com/FlameInTheDark/emerald/internal/config"
	"github.com/FlameInTheDark/emerald/internal/crypto"
	"github.com/FlameInTheDark/emerald/internal/db"
	"github.com/FlameInTheDark/emerald/internal/db/query"
	"github.com/FlameInTheDark/emerald/internal/node"
	"github.com/FlameInTheDark/emerald/internal/node/action"
	"github.com/FlameInTheDark/emerald/internal/node/logic"
	"github.com/FlameInTheDark/emerald/internal/node/lua"
	"github.com/FlameInTheDark/emerald/internal/node/trigger"
	"github.com/FlameInTheDark/emerald/internal/nodedefs"
	"github.com/FlameInTheDark/emerald/internal/pipeline"
	"github.com/FlameInTheDark/emerald/internal/pipelineops"
	"github.com/FlameInTheDark/emerald/internal/plugins"
	"github.com/FlameInTheDark/emerald/internal/scheduler"
	"github.com/FlameInTheDark/emerald/internal/shellcmd"
	"github.com/FlameInTheDark/emerald/internal/skills"
	"github.com/FlameInTheDark/emerald/internal/ws"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	database, err := db.New(cfg.Database.Path)
	if err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			log.Printf("failed to close database: %v", err)
		}
	}()

	if err := db.Migrate(database); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	appConfigStore := query.NewAppConfigStore(database.DB)
	encryptionKey, err := appConfigStore.EnsureEncryptionKey(context.Background(), cfg.EncryptionKey)
	if err != nil {
		log.Fatalf("failed to initialize app encryption key: %v", err)
	}

	encryptor, err := crypto.NewEncryptor(encryptionKey)
	if err != nil {
		log.Fatalf("failed to initialize encryptor: %v", err)
	}

	clusterStore := query.NewClusterStore(database.DB, encryptor)
	kubernetesClusterStore := query.NewKubernetesClusterStore(database.DB, encryptor)
	llmProviderStore := query.NewLLMProviderStore(database.DB, encryptor)
	channelStore := query.NewChannelStore(database.DB, encryptor)
	channelContactStore := query.NewChannelContactStore(database.DB)
	userStore := query.NewUserStore(database.DB, encryptor)
	secretStore := query.NewSecretStore(database.DB, encryptor)
	pipelineStore := query.NewPipelineStore(database.DB)
	executionStore := query.NewExecutionStore(database.DB)
	if err := userStore.EnsureDefaultUser(context.Background(), cfg.Auth.Username, cfg.Auth.Password); err != nil {
		log.Fatalf("failed to ensure default user: %v", err)
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

	skillsDir, err := skills.ResolveDirectory(os.Getenv("AUTOMATOR_SKILLS_DIR"), workingDir, executablePath)
	if err != nil {
		log.Printf("failed to resolve skills directory: %v", err)
		skillsDir = filepath.Join(workingDir, ".agents", "skills")
	}
	if err := skills.EnsureBundledDefaults(skillsDir); err != nil {
		log.Printf("failed to seed bundled skills: %v", err)
	}
	log.Printf("loading local skills from %s", skillsDir)

	pluginsDir, err := plugins.ResolveDirectory(os.Getenv("AUTOMATOR_PLUGINS_DIR"), workingDir, executablePath)
	if err != nil {
		log.Printf("failed to resolve plugins directory: %v", err)
		pluginsDir = filepath.Join(workingDir, ".agents", "plugins")
	}
	log.Printf("loading local plugins from %s", pluginsDir)

	pluginManager := plugins.NewManager(pluginsDir)
	if err := pluginManager.Refresh(context.Background()); err != nil {
		log.Printf("failed to load plugins: %v", err)
	}
	defer pluginManager.Stop()

	nodeDefinitionService := nodedefs.NewService(pluginManager)

	skillStore := skills.NewStore(skillsDir, 2*time.Second)
	if err := skillStore.Start(context.Background()); err != nil {
		log.Printf("failed to start skill store: %v", err)
	}
	defer skillStore.Stop()

	shellRunner := shellcmd.NewRunner(workingDir)

	var engine *pipeline.Engine
	var executionRunner *pipeline.ExecutionRunner
	channelService := channels.NewService(channelStore, channelContactStore, func(ctx context.Context, event trigger.ChannelEvent) error {
		if engine == nil || executionRunner == nil {
			return nil
		}

		eventCtx := trigger.WithChannelEvent(ctx, event)
		activePipelines, err := pipelineStore.ListActive(eventCtx)
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
			if !pipeline.HasMatchingRootTrigger(eventCtx, *flowData, "channel") {
				continue
			}

			result, err := executionRunner.Run(eventCtx, pipelineModel.ID, *flowData, "channel", executionContext)
			if err != nil {
				log.Printf("channel pipeline %s execution failed: %v", pipelineModel.ID, err)
				continue
			}
			if result.Status == "failed" && result.Error != nil {
				log.Printf("channel pipeline %s execution failed: %v", pipelineModel.ID, result.Error)
			}
		}

		return nil
	})

	registry := node.NewRegistry()
	registry.Register(node.TypeTriggerManual, &trigger.ManualTrigger{})
	registry.Register(node.TypeTriggerCron, &trigger.CronTrigger{})
	registry.Register(node.TypeTriggerWebhook, &trigger.WebhookTrigger{})
	registry.Register(node.TypeTriggerChannel, &trigger.ChannelMessageTrigger{})
	registry.Register(node.TypeActionListNodes, &action.ListNodesAction{Clusters: clusterStore})
	registry.Register(node.TypeActionListVMsCTs, &action.ListVMsCTsAction{Clusters: clusterStore})
	registry.Register(node.TypeActionVMStart, &action.VMStartAction{Clusters: clusterStore})
	registry.Register(node.TypeActionVMStop, &action.VMStopAction{Clusters: clusterStore})
	registry.Register(node.TypeActionVMClone, &action.VMCloneAction{Clusters: clusterStore})
	registry.Register(node.TypeActionKubernetesAPIResources, action.NewKubernetesActionNode(kubernetesClusterStore, action.KubernetesOperationAPIResources))
	registry.Register(node.TypeActionKubernetesListResources, action.NewKubernetesActionNode(kubernetesClusterStore, action.KubernetesOperationListResources))
	registry.Register(node.TypeActionKubernetesGetResource, action.NewKubernetesActionNode(kubernetesClusterStore, action.KubernetesOperationGetResource))
	registry.Register(node.TypeActionKubernetesApplyManifest, action.NewKubernetesActionNode(kubernetesClusterStore, action.KubernetesOperationApplyManifest))
	registry.Register(node.TypeActionKubernetesPatchResource, action.NewKubernetesActionNode(kubernetesClusterStore, action.KubernetesOperationPatchResource))
	registry.Register(node.TypeActionKubernetesDeleteResource, action.NewKubernetesActionNode(kubernetesClusterStore, action.KubernetesOperationDeleteResource))
	registry.Register(node.TypeActionKubernetesScaleResource, action.NewKubernetesActionNode(kubernetesClusterStore, action.KubernetesOperationScaleResource))
	registry.Register(node.TypeActionKubernetesRolloutRestart, action.NewKubernetesActionNode(kubernetesClusterStore, action.KubernetesOperationRolloutRestart))
	registry.Register(node.TypeActionKubernetesRolloutStatus, action.NewKubernetesActionNode(kubernetesClusterStore, action.KubernetesOperationRolloutStatus))
	registry.Register(node.TypeActionKubernetesPodLogs, action.NewKubernetesActionNode(kubernetesClusterStore, action.KubernetesOperationPodLogs))
	registry.Register(node.TypeActionKubernetesPodExec, action.NewKubernetesActionNode(kubernetesClusterStore, action.KubernetesOperationPodExec))
	registry.Register(node.TypeActionKubernetesEvents, action.NewKubernetesActionNode(kubernetesClusterStore, action.KubernetesOperationEvents))
	registry.Register(node.TypeActionHTTP, &action.HTTPAction{})
	registry.Register(node.TypeActionShell, &action.ShellCommandAction{Runner: shellRunner})
	registry.Register(node.TypeActionChannelSend, &action.ChannelSendAction{
		Channels: channelStore,
		Contacts: channelContactStore,
		Sender:   channelService,
	})
	registry.Register(node.TypeActionChannelReply, &action.ChannelReplyAction{
		Channels: channelStore,
		Contacts: channelContactStore,
		Sender:   channelService,
	})
	registry.Register(node.TypeActionChannelEdit, &action.ChannelEditAction{
		Channels: channelStore,
		Contacts: channelContactStore,
		Sender:   channelService,
	})
	registry.Register(node.TypeActionChannelWait, &action.ChannelSendAndWaitAction{
		Channels: channelStore,
		Contacts: channelContactStore,
		Sender:   channelService,
		Waiter:   channelService,
	})
	registry.Register(node.TypeActionGetPipeline, &action.GetPipelineAction{Pipelines: pipelineStore})
	registry.Register(node.TypeLogicReturn, &logic.ReturnNode{})
	registry.Register(node.TypeLogicCondition, &logic.ConditionNode{})
	registry.Register(node.TypeLogicSwitch, &logic.SwitchNode{})
	registry.Register(node.TypeLogicMerge, &logic.MergeNode{})
	registry.Register(node.TypeLogicAggregate, &logic.AggregateNode{})
	llmPromptNode := &logic.LLMPromptNode{Providers: llmProviderStore}
	registry.Register(node.TypeLLMPrompt, llmPromptNode)
	registry.Register(node.TypeLLMPromptLegacy, llmPromptNode)
	registry.Register(node.TypeLLMAgent, &logic.LLMAgentNode{Providers: llmProviderStore, Skills: skillStore})
	registry.Register(node.TypeActionLua, &lua.LuaNode{})

	engine = pipeline.NewEngine(registry)
	wsHub := ws.NewHub()
	go wsHub.Run()
	executionRunner = pipeline.NewExecutionRunner(
		executionStore,
		engine,
		wsHub,
		pipeline.WithFlowSemanticValidator(nodeDefinitionService),
		pipeline.WithSecretTemplateValueProvider(secretStore),
	)
	pipelineInvoker := pipeline.NewInvoker(database.DB, pipelineStore, engine, executionRunner)
	pipelineRunner := func(ctx context.Context, pipelineID string) error {
		flowData, err := scheduler.LoadFlowData(database.DB, pipelineID)
		if err != nil {
			return err
		}
		result, err := executionRunner.Run(ctx, pipelineID, *flowData, "cron", nil)
		if err != nil {
			return err
		}
		if result.Status == "failed" && result.Error != nil {
			return result.Error
		}
		return nil
	}
	cronScheduler := scheduler.New(database.DB, pipelineRunner)
	pipelineManager := pipelineops.NewService(pipelineStore, cronScheduler, nodeDefinitionService)
	registry.Register(node.TypeToolListNodes, &action.ListNodesToolNode{Clusters: clusterStore})
	registry.Register(node.TypeToolListVMsCTs, &action.ListVMsCTsToolNode{Clusters: clusterStore})
	registry.Register(node.TypeToolVMStart, &action.VMStartToolNode{Clusters: clusterStore})
	registry.Register(node.TypeToolVMStop, &action.VMStopToolNode{Clusters: clusterStore})
	registry.Register(node.TypeToolVMClone, &action.VMCloneToolNode{Clusters: clusterStore})
	registry.Register(node.TypeToolKubernetesAPIResources, action.NewKubernetesToolNode(kubernetesClusterStore, action.KubernetesOperationAPIResources))
	registry.Register(node.TypeToolKubernetesListResources, action.NewKubernetesToolNode(kubernetesClusterStore, action.KubernetesOperationListResources))
	registry.Register(node.TypeToolKubernetesGetResource, action.NewKubernetesToolNode(kubernetesClusterStore, action.KubernetesOperationGetResource))
	registry.Register(node.TypeToolKubernetesApplyManifest, action.NewKubernetesToolNode(kubernetesClusterStore, action.KubernetesOperationApplyManifest))
	registry.Register(node.TypeToolKubernetesPatchResource, action.NewKubernetesToolNode(kubernetesClusterStore, action.KubernetesOperationPatchResource))
	registry.Register(node.TypeToolKubernetesDeleteResource, action.NewKubernetesToolNode(kubernetesClusterStore, action.KubernetesOperationDeleteResource))
	registry.Register(node.TypeToolKubernetesScaleResource, action.NewKubernetesToolNode(kubernetesClusterStore, action.KubernetesOperationScaleResource))
	registry.Register(node.TypeToolKubernetesRolloutRestart, action.NewKubernetesToolNode(kubernetesClusterStore, action.KubernetesOperationRolloutRestart))
	registry.Register(node.TypeToolKubernetesRolloutStatus, action.NewKubernetesToolNode(kubernetesClusterStore, action.KubernetesOperationRolloutStatus))
	registry.Register(node.TypeToolKubernetesPodLogs, action.NewKubernetesToolNode(kubernetesClusterStore, action.KubernetesOperationPodLogs))
	registry.Register(node.TypeToolKubernetesPodExec, action.NewKubernetesToolNode(kubernetesClusterStore, action.KubernetesOperationPodExec))
	registry.Register(node.TypeToolKubernetesEvents, action.NewKubernetesToolNode(kubernetesClusterStore, action.KubernetesOperationEvents))
	registry.Register(node.TypeToolHTTP, &action.HTTPToolNode{})
	registry.Register(node.TypeToolShell, &action.ShellCommandToolNode{Runner: shellRunner})
	registry.Register(node.TypeToolChannelWait, &action.ChannelSendAndWaitToolNode{
		Channels: channelStore,
		Contacts: channelContactStore,
		Sender:   channelService,
		Waiter:   channelService,
	})
	registry.Register(node.TypeActionRunPipeline, &action.RunPipelineAction{Runner: pipelineInvoker})
	registry.Register(node.TypeToolListPipelines, &action.PipelineListToolNode{Pipelines: pipelineStore})
	registry.Register(node.TypeToolGetPipeline, &action.PipelineGetToolNode{Pipelines: pipelineStore})
	registry.Register(node.TypeToolCreatePipeline, &action.PipelineCreateToolNode{Manager: pipelineManager})
	registry.Register(node.TypeToolUpdatePipeline, &action.PipelineUpdateToolNode{Manager: pipelineManager})
	registry.Register(node.TypeToolDeletePipeline, &action.PipelineDeleteToolNode{Manager: pipelineManager})
	registry.Register(node.TypeToolRunPipeline, &action.PipelineRunToolNode{
		Pipelines: pipelineStore,
		Runner:    pipelineInvoker,
	})
	for _, binding := range pluginManager.Bindings() {
		nodeType := node.NodeType(binding.Type)
		switch binding.Kind {
		case "tool":
			registry.Register(nodeType, &plugins.ToolExecutor{
				Manager:  pluginManager,
				NodeType: binding.Type,
			})
		default:
			registry.Register(nodeType, &plugins.ActionExecutor{
				Manager:  pluginManager,
				NodeType: binding.Type,
				Outputs:  binding.Spec.Outputs,
			})
		}
	}

	cronScheduler.Start()
	defer cronScheduler.Stop()

	if err := channelService.Start(); err != nil {
		log.Printf("failed to start channel service: %v", err)
	}
	defer channelService.Stop()

	authService := auth.NewService(userStore, auth.Config{
		SessionTTL: cfg.Auth.SessionTTL,
		CookieName: cfg.Auth.CookieName,
	})

	app := api.New(api.Config{
		DB:              database,
		Scheduler:       cronScheduler,
		ChannelService:  channelService,
		EncryptionKey:   encryptionKey,
		ExecutionRunner: executionRunner,
		WSHub:           wsHub,
		SkillStore:      skillStore,
		ShellRunner:     shellRunner,
		AuthService:     authService,
		NodeDefinitions: nodeDefinitionService,
		SecretStore:     secretStore,
	})

	go func() {
		if err := app.Listen(":" + cfg.Server.Port); err != nil {
			log.Fatalf("server failed: %v", err)
		}
	}()

	log.Printf("server started on port %s", cfg.Server.Port)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down server...")
}
