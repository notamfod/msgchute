package registry

import (
	"github.com/jmoiron/sqlx"

	"github.com/devian2011/msgchute/internal/handler/admin"
	"github.com/devian2011/msgchute/internal/handler/public"
	"github.com/devian2011/msgchute/internal/io/web"
	"github.com/devian2011/msgchute/internal/service/auth"
	"github.com/devian2011/msgchute/internal/service/event"
	"github.com/devian2011/msgchute/internal/service/sender"
	"github.com/devian2011/msgchute/internal/service/stoplist"
	"github.com/devian2011/msgchute/internal/service/template"
)

type AppRegistry struct {
	DB   *sqlx.DB
	Http *web.Server

	AuthProvider *auth.Provider

	Services *Services
	Handlers *Handlers

	Middlewares *Middlewares
}

type Middlewares struct {
	Auth *auth.HttpMiddleware
}

type Services struct {
	Sender      *sender.Sender
	SenderQueue *sender.Queue
	EventBus    *event.Bus
}

type Handlers struct {
	Public *PublicHandlers
	Admin  *AdminHandlers
}

type PublicHandlers struct {
	TemplateMetadata *template.Manager
	Transports       []sender.Transport
	Sender           *public.SenderHandler
	BatchSender      *public.BatchSenderHandler
	Retrier          *public.RetryHandler
	Preview          *public.PreviewHandler
}

type AdminHandlers struct {
	StopList        *stoplist.Service
	TemplateCreator *admin.TemplateCreateHandler
	TemplateUpdater *admin.TemplateUpdateHandler
	TemplateFinder  *admin.TemplateFinderHandler
	TemplateDeleter *template.Manager

	MessageFinder          *admin.MessageFindHandler
	MessageFindByID        *admin.MessageFindByIDHandler
	MessageDictionary      *admin.MessageDictionaryHandler
	MessageRecipientFinder *admin.MessageRecipientFindHandler

	WorkerStatus *admin.WorkerStatusHandler
}
