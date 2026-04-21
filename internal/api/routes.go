package api

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/FlameInTheDark/emerald/internal/api/handlers"
	"github.com/FlameInTheDark/emerald/internal/assistants"
	"github.com/FlameInTheDark/emerald/internal/auth"
	"github.com/FlameInTheDark/emerald/internal/channels"
	appconfig "github.com/FlameInTheDark/emerald/internal/config"
	"github.com/FlameInTheDark/emerald/internal/crypto"
	"github.com/FlameInTheDark/emerald/internal/db"
	"github.com/FlameInTheDark/emerald/internal/db/query"
	"github.com/FlameInTheDark/emerald/internal/nodedefs"
	"github.com/FlameInTheDark/emerald/internal/pipeline"
	"github.com/FlameInTheDark/emerald/internal/pipelineops"
	"github.com/FlameInTheDark/emerald/internal/scheduler"
	"github.com/FlameInTheDark/emerald/internal/shellcmd"
	"github.com/FlameInTheDark/emerald/internal/skills"
	"github.com/FlameInTheDark/emerald/internal/templateops"
	"github.com/FlameInTheDark/emerald/internal/triggers"
	"github.com/FlameInTheDark/emerald/internal/webtools"
	"github.com/FlameInTheDark/emerald/internal/ws"
)

//go:embed web/dist
var embeddedFS embed.FS

type Config struct {
	DB              *db.DB
	Scheduler       *scheduler.Scheduler
	ChannelService  *channels.Service
	EncryptionKey   string
	ExecutionRunner *pipeline.ExecutionRunner
	WSHub           *ws.Hub
	SkillStore      skills.Reader
	ShellRunner     shellcmd.Runner
	AuthService     *auth.Service
	NodeDefinitions *nodedefs.Service
	SecretStore     *query.SecretStore
	TriggerService  *triggers.Service
	AuditLogStore   *query.AuditLogStore
	Security        appconfig.SecurityConfig
}

func New(cfg Config) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:                 "Emerald",
		ServerHeader:            "Emerald",
		EnableTrustedProxyCheck: cfg.Security.TrustProxy,
		TrustedProxies:          append([]string(nil), cfg.Security.TrustedProxies...),
	})

	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(securityHeadersMiddleware())
	if len(cfg.Security.AllowedOrigins) > 0 {
		app.Use(cors.New(cors.Config{
			AllowOrigins:     strings.Join(cfg.Security.AllowedOrigins, ","),
			AllowMethods:     "GET,POST,PUT,DELETE,OPTIONS",
			AllowHeaders:     "Origin,Content-Type,Accept,Authorization",
			AllowCredentials: true,
		}))
	}

	authService := cfg.AuthService
	if authService == nil {
		authService = auth.NewService(nil, auth.Config{})
	}

	var encryptor *crypto.Encryptor
	if cfg.EncryptionKey != "" {
		var err error
		encryptor, err = crypto.NewEncryptor(cfg.EncryptionKey)
		if err != nil {
			log.Printf("warning: failed to create encryptor: %v", err)
		}
	}

	wsHub := cfg.WSHub
	if wsHub == nil {
		wsHub = ws.NewHub()
		go wsHub.Run()
	}

	clusterStore := query.NewClusterStore(cfg.DB.DB, encryptor)
	clusterHandler := handlers.NewClusterHandler(clusterStore)
	kubernetesClusterStore := query.NewKubernetesClusterStore(cfg.DB.DB, encryptor)
	kubernetesClusterHandler := handlers.NewKubernetesClusterHandler(kubernetesClusterStore, handlers.KubernetesClusterHandlerOptions{
		AuthService:   authService,
		AuditLogStore: cfg.AuditLogStore,
	})
	channelStore := query.NewChannelStore(cfg.DB.DB, encryptor)
	channelContactStore := query.NewChannelContactStore(cfg.DB.DB)
	channelHandler := handlers.NewChannelHandler(channelStore, channelContactStore, cfg.ChannelService, handlers.ChannelHandlerOptions{
		AuthService:   authService,
		AuditLogStore: cfg.AuditLogStore,
	})
	userStore := query.NewUserStore(cfg.DB.DB, encryptor)
	secretStore := cfg.SecretStore
	if secretStore == nil {
		secretStore = query.NewSecretStore(cfg.DB.DB, encryptor)
	}
	secretHandler := handlers.NewSecretHandlerWithOptions(secretStore, handlers.SecretHandlerOptions{
		AuthService:   authService,
		AuditLogStore: cfg.AuditLogStore,
	})
	appConfigStore := query.NewAppConfigStore(cfg.DB.DB)
	assistantProfileStore := assistants.NewStore(appConfigStore)
	webToolsStore := webtools.NewStore(appConfigStore, secretStore, cfg.Security.AllowPrivateWebTools)

	pipelineStore := query.NewPipelineStore(cfg.DB.DB)
	templateStore := query.NewTemplateStore(cfg.DB.DB)
	llmProviderStore := query.NewLLMProviderStore(cfg.DB.DB, encryptor)
	chatStore := query.NewChatStore(cfg.DB.DB)
	executionStore := query.NewExecutionStore(cfg.DB.DB)
	nodeDefinitionService := cfg.NodeDefinitions
	if nodeDefinitionService == nil {
		nodeDefinitionService = nodedefs.NewService(nil)
	}
	nodeDefinitionsHandler := handlers.NewNodeDefinitionsHandler(nodeDefinitionService, cfg.TriggerService)
	llmProviderHandler := handlers.NewLLMProviderHandler(llmProviderStore, handlers.LLMProviderHandlerOptions{
		AuthService:   authService,
		AuditLogStore: cfg.AuditLogStore,
	})
	dashboardHandler := handlers.NewDashboardHandler(clusterStore, pipelineStore, executionStore, channelStore, cfg.Scheduler)
	authHandler := handlers.NewAuthHandler(authService, handlers.AuthHandlerOptions{TrustProxy: cfg.Security.TrustProxy})
	userHandler := handlers.NewUserHandler(userStore, authService, handlers.UserHandlerOptions{TrustProxy: cfg.Security.TrustProxy})
	var pipelineReloader interface {
		Reload(ctx context.Context) error
	}
	if cfg.TriggerService != nil {
		pipelineReloader = cfg.TriggerService
	} else if cfg.Scheduler != nil {
		pipelineReloader = cfg.Scheduler
	}
	pipelineService := pipelineops.NewService(pipelineStore, pipelineReloader, nodeDefinitionService)
	if cfg.TriggerService != nil {
		pipelineService.SetActivationValidator(cfg.TriggerService)
	}
	templateHandler := handlers.NewTemplateHandler(templateops.NewService(templateStore, pipelineService, nodeDefinitionService))

	pipelineRunHandler := handlers.NewPipelineRunHandler(
		pipelineStore,
		cfg.ExecutionRunner,
	)
	webhookHandler := handlers.NewWebhookHandler(cfg.TriggerService)
	llmChatHandler := handlers.NewLLMChatHandler(llmProviderStore, clusterStore, kubernetesClusterStore, pipelineStore, chatStore, cfg.ExecutionRunner, cfg.Scheduler, cfg.SkillStore, cfg.ShellRunner, assistantProfileStore, webToolsStore, handlers.LLMChatHandlerOptions{
		AuditLogStore: cfg.AuditLogStore,
	})
	editorAssistantHandler := handlers.NewEditorAssistantHandler(llmProviderStore, assistantProfileStore, cfg.SkillStore, nodeDefinitionService)
	assistantProfileHandler := handlers.NewAssistantProfileHandler(assistantProfileStore)
	webToolsHandler := handlers.NewWebToolsHandler(webToolsStore)
	executionHandler := handlers.NewExecutionHandler(executionStore, cfg.ExecutionRunner)

	api := app.Group("/api/v1")
	api.Use(stateChangingOriginMiddleware(cfg.Security))

	api.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})
	api.Post("/auth/login", authHandler.Login)
	api.Get("/auth/session", authHandler.Session)
	api.Post("/auth/logout", authHandler.Logout)

	app.All("/webhook", webhookHandler.Handle)
	app.All("/webhook/*", webhookHandler.Handle)

	api.Post("/channels/connect", channelHandler.Connect)
	api.Use(authMiddleware(authService, cfg.Security.TrustProxy))

	api.Get("/dashboard/stats", dashboardHandler.Stats)
	api.Get("/node-definitions", nodeDefinitionsHandler.List)
	api.Post("/node-definitions/refresh", nodeDefinitionsHandler.Refresh)

	clusters := api.Group("/clusters")
	clusters.Get("/", clusterHandler.List)
	clusters.Post("/", clusterHandler.Create)
	clusters.Get("/:id", clusterHandler.Get)
	clusters.Post("/:id/reveal", clusterHandler.Reveal)
	clusters.Put("/:id", clusterHandler.Update)
	clusters.Delete("/:id", clusterHandler.Delete)

	kubernetesClusters := api.Group("/kubernetes/clusters")
	kubernetesClusters.Get("/", kubernetesClusterHandler.List)
	kubernetesClusters.Post("/", kubernetesClusterHandler.Create)
	kubernetesClusters.Post("/test", kubernetesClusterHandler.Test)
	kubernetesClusters.Get("/:id", kubernetesClusterHandler.Get)
	kubernetesClusters.Post("/:id/reveal", kubernetesClusterHandler.Reveal)
	kubernetesClusters.Put("/:id", kubernetesClusterHandler.Update)
	kubernetesClusters.Delete("/:id", kubernetesClusterHandler.Delete)

	channelRoutes := api.Group("/channels")
	channelRoutes.Get("/", channelHandler.List)
	channelRoutes.Post("/", channelHandler.Create)
	channelRoutes.Get("/:id", channelHandler.Get)
	channelRoutes.Post("/:id/reveal", channelHandler.Reveal)
	channelRoutes.Put("/:id", channelHandler.Update)
	channelRoutes.Delete("/:id", channelHandler.Delete)
	channelRoutes.Get("/:id/contacts", channelHandler.ListContacts)

	pipelineHandler := handlers.NewPipelineHandler(pipelineStore, pipelineReloader, nodeDefinitionService)
	if cfg.TriggerService != nil {
		pipelineHandler.SetActivationValidator(cfg.TriggerService)
	}
	pipelines := api.Group("/pipelines")
	pipelines.Get("/", pipelineHandler.List)
	pipelines.Post("/", pipelineHandler.Create)
	pipelines.Get("/:id", pipelineHandler.Get)
	pipelines.Put("/:id", pipelineHandler.Update)
	pipelines.Delete("/:id", pipelineHandler.Delete)
	pipelines.Get("/:id/export", pipelineHandler.Export)
	pipelines.Post("/:id/run", pipelineRunHandler.Run)

	templates := api.Group("/templates")
	templates.Get("/", templateHandler.List)
	templates.Post("/", templateHandler.Create)
	templates.Post("/import", templateHandler.Import)
	templates.Get("/export", templateHandler.ExportAll)
	templates.Get("/:id", templateHandler.Get)
	templates.Delete("/:id", templateHandler.Delete)
	templates.Post("/:id/clone", templateHandler.Clone)
	templates.Post("/:id/pipelines", templateHandler.CreatePipeline)
	templates.Get("/:id/export", templateHandler.Export)

	llmProviders := api.Group("/llm-providers")
	llmProviders.Get("/", llmProviderHandler.List)
	llmProviders.Post("/", llmProviderHandler.Create)
	llmProviders.Post("/discover-models", llmProviderHandler.DiscoverModels)
	llmProviders.Get("/:id", llmProviderHandler.Get)
	llmProviders.Post("/:id/reveal", llmProviderHandler.Reveal)
	llmProviders.Put("/:id", llmProviderHandler.Update)
	llmProviders.Delete("/:id", llmProviderHandler.Delete)
	llmProviders.Get("/:id/models", llmProviderHandler.ListModels)

	users := api.Group("/users")
	users.Get("/", userHandler.List)
	users.Post("/", userHandler.Create)
	users.Post("/change-password", userHandler.ChangePassword)
	users.Delete("/:id", userHandler.Delete)

	secrets := api.Group("/secrets")
	secrets.Get("/", secretHandler.List)
	secrets.Post("/", secretHandler.Create)
	secrets.Get("/:id", secretHandler.Get)
	secrets.Post("/:id/reveal", secretHandler.Reveal)
	secrets.Put("/:id", secretHandler.Update)
	secrets.Delete("/:id", secretHandler.Delete)

	llmRoutes := api.Group("/llm")
	llmRoutes.Get("/conversations", llmChatHandler.ListConversations)
	llmRoutes.Get("/conversations/:id", llmChatHandler.GetConversation)
	llmRoutes.Put("/conversations/:id", llmChatHandler.UpdateConversation)
	llmRoutes.Delete("/conversations/:id", llmChatHandler.DeleteConversation)
	llmRoutes.Post("/chat/stream", llmChatHandler.ChatStream)
	llmRoutes.Post("/chat", llmChatHandler.Chat)
	llmRoutes.Post("/editor-assistant/stream", editorAssistantHandler.ChatStream)

	assistantProfiles := api.Group("/assistant-profiles")
	assistantProfiles.Get("/:scope", assistantProfileHandler.Get)
	assistantProfiles.Put("/:scope", assistantProfileHandler.Update)
	assistantProfiles.Post("/:scope/restore-defaults", assistantProfileHandler.RestoreDefaults)

	webToolRoutes := api.Group("/web-tools")
	webToolRoutes.Get("/config", webToolsHandler.Get)
	webToolRoutes.Put("/config", webToolsHandler.Update)

	executions := api.Group("/executions")
	executions.Get("/pipelines/:id", executionHandler.ListByPipeline)
	executions.Get("/pipelines/:id/active", executionHandler.ListActiveByPipeline)
	executions.Get("/:executionId", executionHandler.Get)
	executions.Post("/:executionId/cancel", executionHandler.Cancel)

	app.Get("/ws/:channel", websocketOriginMiddleware(cfg.Security), websocketAuthMiddleware(authService, cfg.Security.TrustProxy), ws.WSUpgrader(wsHub))

	app.Use("*", serveEmbedded())

	return app
}
func serveEmbedded() fiber.Handler {
	embedded, err := fs.Sub(embeddedFS, "web/dist")
	if err != nil {
		panic(err)
	}

	return filesystem.New(filesystem.Config{
		Root:         http.FS(embedded),
		Index:        "index.html",
		NotFoundFile: "index.html",
	})
}
