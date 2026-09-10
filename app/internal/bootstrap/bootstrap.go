package bootstrap

import (
	"context"
	"fmt"

	"github.com/devian2011/retrier"

	"github.com/devian2011/msgchute/internal/data/repository"
	"github.com/devian2011/msgchute/internal/handler/admin"
	"github.com/devian2011/msgchute/internal/handler/public"
	"github.com/devian2011/msgchute/internal/io/storage"
	"github.com/devian2011/msgchute/internal/io/web"
	"github.com/devian2011/msgchute/internal/registry"
	"github.com/devian2011/msgchute/internal/service/auth"
	"github.com/devian2011/msgchute/internal/service/event"
	"github.com/devian2011/msgchute/internal/service/message"
	"github.com/devian2011/msgchute/internal/service/sender"
	"github.com/devian2011/msgchute/internal/service/template"
)

func Bootstrap(ctx context.Context, cfgFilePath string) (*registry.AppRegistry, error) {
	cfg, loadCfgErr := loadConfig(cfgFilePath)
	if loadCfgErr != nil {
		return nil, fmt.Errorf("load config: %w", loadCfgErr)
	}
	// DB init and migrate
	db, dbConnectErr := storage.NewDB(cfg.Db)
	if dbConnectErr != nil {
		return nil, fmt.Errorf("connect db: %w", dbConnectErr)
	}

	migrateErr := storage.Migrate(cfg.Db)
	if migrateErr != nil {
		return nil, fmt.Errorf("migrate db: %w", migrateErr)
	}

	msgRepo := repository.NewMessageRepository(db)
	msgTemplateRepo := repository.NewMessageTemplateRepository(db)
	taskRepo := repository.NewTaskRepository(db)
	taskResultRepo := repository.NewTaskResultRepository(db)

	// Auth block
	authProvider, authMiddleware, authProviderErr := initAuth(ctx, cfg.Auth)
	if authProviderErr != nil {
		return nil, fmt.Errorf("init auth: %w", authProviderErr)
	}
	// Http block
	httpSrv := web.NewServer(cfg.Http)

	// Message services
	msgStatusUpdater := message.NewStatusUpdater(db, msgRepo, taskRepo)

	// Init sender event bus
	eventBus := event.NewBus(ctx, msgStatusUpdater)

	// Generator Service
	strGenerator, genInitErr := template.NewGenerator()
	if genInitErr != nil {
		return nil, fmt.Errorf("create generator: %w", genInitErr)
	}
	tmplMgr := template.NewManager(db, strGenerator, msgTemplateRepo)

	// Sender services init
	providerManager := sender.NewProviderManager(ctx, cfg.Providers.PluginMap)
	workerStore := sender.NewWorkerStore(ctx, db, taskResultRepo, taskRepo, msgRepo)
	workerManager := retrier.NewManager(
		ctx, workerStore, &sender.Logger{}, retrier.NewBackOffStrategy(),
		cfg.Providers.MaxBufferSize, cfg.Providers.FetchTaskTimeout, cfg.Providers.FetchTaskTimeoutMax,
		eventBus)

	// Message finder
	msgFinder := message.NewFinder(db, msgRepo, taskRepo, taskResultRepo)

	// msgSender
	msgSender := sender.NewSender(ctx, cfg.Providers, providerManager, workerManager, tmplMgr)
	msgQueue := sender.NewQueue(ctx, db, taskRepo, msgRepo, cfg.Providers.Providers, tmplMgr)

	return &registry.AppRegistry{
		DB:           db,
		Http:         httpSrv,
		AuthProvider: authProvider,
		Middlewares: &registry.Middlewares{
			Auth: authMiddleware,
		},
		Services: &registry.Services{
			EventBus:    eventBus,
			Sender:      msgSender,
			SenderQueue: msgQueue,
		},
		Handlers: &registry.Handlers{
			Public: &registry.PublicHandlers{
				Sender:      public.NewSenderHandler(msgQueue),
				BatchSender: public.NewBatchSenderHandler(msgQueue),
				Preview:     public.NewPreviewHandler(tmplMgr),
				Retrier:     public.NewMessageRetryHandler(msgQueue),
			},
			Admin: &registry.AdminHandlers{
				TemplateCreator: admin.NewTemplateCreateHandler(tmplMgr),
				TemplateUpdater: admin.NewTemplateUpdateHandler(tmplMgr),
				TemplateFinder:  admin.NewTemplateFinderHandler(tmplMgr),

				MessageFinder:          admin.NewMessageFindHandler(msgFinder),
				MessageFindByID:        admin.NewMessageFindByIDHandler(msgFinder),
				MessageDictionary:      admin.NewMessageDictionaryHandler(msgFinder),
				MessageRecipientFinder: admin.NewMessageRecipientFindHandler(msgFinder),

				WorkerStatus: admin.NewWorkerHandler(workerManager),
			},
		},
	}, nil
}

func initAuth(ctx context.Context, cfg *auth.Config) (*auth.Provider, *auth.HttpMiddleware, error) {
	if cfg == nil || len(cfg.Plugin) == 0 {
		return nil, auth.NewMiddleware(&auth.EmptyAuthProvider{}), nil
	}

	apClient, apInitErr := auth.NewProvider(ctx, cfg)
	if apInitErr != nil {
		return nil, nil, apInitErr
	}

	return apClient, auth.NewMiddleware(apClient.GetProvider()), nil
}
