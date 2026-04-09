package api

import (
	"embed"
	"io/fs"
	"log"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/FlameInTheDark/automator/internal/api/handlers"
	"github.com/FlameInTheDark/automator/internal/assistants"
	"github.com/FlameInTheDark/automator/internal/auth"
	"github.com/FlameInTheDark/automator/internal/channels"
	"github.com/FlameInTheDark/automator/internal/crypto"
	"github.com/FlameInTheDark/automator/internal/db"
	"github.com/FlameInTheDark/automator/internal/db/query"
	"github.com/FlameInTheDark/automator/internal/pipeline"
	"github.com/FlameInTheDark/automator/internal/pipelineops"
	"github.com/FlameInTheDark/automator/internal/scheduler"
	"github.com/FlameInTheDark/automator/internal/shellcmd"
	"github.com/FlameInTheDark/automator/internal/skills"
	"github.com/FlameInTheDark/automator/internal/templateops"
	"github.com/FlameInTheDark/automator/internal/ws"
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
}

func New(cfg Config) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:      "Proxmox Automator",
		ServerHeader: "Automator",
	})

	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders: "Origin,Content-Type,Accept,Authorization",
	}))

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
	kubernetesClusterHandler := handlers.NewKubernetesClusterHandler(kubernetesClusterStore)
	channelStore := query.NewChannelStore(cfg.DB.DB, encryptor)
	channelContactStore := query.NewChannelContactStore(cfg.DB.DB)
	channelHandler := handlers.NewChannelHandler(channelStore, channelContactStore, cfg.ChannelService)
	userStore := query.NewUserStore(cfg.DB.DB, encryptor)
	appConfigStore := query.NewAppConfigStore(cfg.DB.DB)
	assistantProfileStore := assistants.NewStore(appConfigStore)

	pipelineStore := query.NewPipelineStore(cfg.DB.DB)
	templateStore := query.NewTemplateStore(cfg.DB.DB)
	llmProviderStore := query.NewLLMProviderStore(cfg.DB.DB, encryptor)
	chatStore := query.NewChatStore(cfg.DB.DB)
	executionStore := query.NewExecutionStore(cfg.DB.DB)
	llmProviderHandler := handlers.NewLLMProviderHandler(llmProviderStore)
	dashboardHandler := handlers.NewDashboardHandler(clusterStore, pipelineStore, executionStore, channelStore, cfg.Scheduler)
	authHandler := handlers.NewAuthHandler(authService)
	userHandler := handlers.NewUserHandler(userStore, authService)
	pipelineService := pipelineops.NewService(pipelineStore, cfg.Scheduler)
	templateHandler := handlers.NewTemplateHandler(templateops.NewService(templateStore, pipelineService))

	pipelineRunHandler := handlers.NewPipelineRunHandler(
		pipelineStore,
		cfg.ExecutionRunner,
	)
	llmChatHandler := handlers.NewLLMChatHandler(llmProviderStore, clusterStore, kubernetesClusterStore, pipelineStore, chatStore, cfg.ExecutionRunner, cfg.Scheduler, cfg.SkillStore, cfg.ShellRunner, assistantProfileStore)
	editorAssistantHandler := handlers.NewEditorAssistantHandler(llmProviderStore, assistantProfileStore)
	assistantProfileHandler := handlers.NewAssistantProfileHandler(assistantProfileStore)
	executionHandler := handlers.NewExecutionHandler(executionStore, cfg.ExecutionRunner)

	api := app.Group("/api/v1")

	api.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})
	api.Post("/auth/login", authHandler.Login)
	api.Get("/auth/session", authHandler.Session)
	api.Post("/auth/logout", authHandler.Logout)

	api.Post("/channels/connect", channelHandler.Connect)
	api.Use(authMiddleware(authService))

	api.Get("/dashboard/stats", dashboardHandler.Stats)

	clusters := api.Group("/clusters")
	clusters.Get("/", clusterHandler.List)
	clusters.Post("/", clusterHandler.Create)
	clusters.Get("/:id", clusterHandler.Get)
	clusters.Put("/:id", clusterHandler.Update)
	clusters.Delete("/:id", clusterHandler.Delete)

	kubernetesClusters := api.Group("/kubernetes/clusters")
	kubernetesClusters.Get("/", kubernetesClusterHandler.List)
	kubernetesClusters.Post("/", kubernetesClusterHandler.Create)
	kubernetesClusters.Post("/test", kubernetesClusterHandler.Test)
	kubernetesClusters.Get("/:id", kubernetesClusterHandler.Get)
	kubernetesClusters.Put("/:id", kubernetesClusterHandler.Update)
	kubernetesClusters.Delete("/:id", kubernetesClusterHandler.Delete)

	channelRoutes := api.Group("/channels")
	channelRoutes.Get("/", channelHandler.List)
	channelRoutes.Post("/", channelHandler.Create)
	channelRoutes.Get("/:id", channelHandler.Get)
	channelRoutes.Put("/:id", channelHandler.Update)
	channelRoutes.Delete("/:id", channelHandler.Delete)
	channelRoutes.Get("/:id/contacts", channelHandler.ListContacts)

	pipelineHandler := pipelineHandler(pipelineStore, cfg.Scheduler)
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
	llmProviders.Get("/:id", llmProviderHandler.Get)
	llmProviders.Put("/:id", llmProviderHandler.Update)
	llmProviders.Delete("/:id", llmProviderHandler.Delete)
	llmProviders.Get("/:id/models", llmProviderHandler.ListModels)

	users := api.Group("/users")
	users.Get("/", userHandler.List)
	users.Post("/", userHandler.Create)
	users.Post("/change-password", userHandler.ChangePassword)
	users.Delete("/:id", userHandler.Delete)

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

	executions := api.Group("/executions")
	executions.Get("/pipelines/:id", executionHandler.ListByPipeline)
	executions.Get("/pipelines/:id/active", executionHandler.ListActiveByPipeline)
	executions.Get("/:executionId", executionHandler.Get)
	executions.Post("/:executionId/cancel", executionHandler.Cancel)

	app.Get("/ws/:channel", websocketAuthMiddleware(authService), ws.WSUpgrader(wsHub))

	app.Use("*", serveEmbedded())

	return app
}

func pipelineHandler(store *query.PipelineStore, scheduler *scheduler.Scheduler) *handlers.PipelineHandler {
	return handlers.NewPipelineHandler(store, scheduler)
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
