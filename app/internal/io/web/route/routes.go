package route

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httplog/v3"
	"github.com/swaggo/http-swagger/v2"

	"github.com/devian2011/msgchute/internal/io/web"
	"github.com/devian2011/msgchute/internal/io/web/endpoint/admin"
	"github.com/devian2011/msgchute/internal/io/web/endpoint/public"
	"github.com/devian2011/msgchute/internal/registry"
)

func RegisterRoutes(s *web.Server, handlers *registry.Handlers, m *registry.Middlewares) {
	r := chi.NewRouter()

	r.Use(
		middleware.RequestID,
		middleware.ClientIPFromRemoteAddr,
		cors.Handler(cors.Options{
			AllowedOrigins:   []string{"*"},
			AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Token", "X-Internal-Token"},
			AllowCredentials: true,
		}),
		httplog.RequestLogger(
			slog.Default(),
			&httplog.Options{
				Level:  slog.LevelInfo,
				Schema: httplog.SchemaOTEL,
			},
		),
		middleware.Recoverer,
		middleware.Timeout(15*time.Second),
		m.Auth.Middleware,
	)

	r.Use(m.Auth.Middleware)

	r.Options("/*", func(w http.ResponseWriter, r *http.Request) {})

	if s.WithSwagger() {
		r.Get("/api/swagger/*", httpSwagger.Handler(
			httpSwagger.URL("/api/swagger/doc.json"),
		))
	}

	dashboard := s.Dashboard()

	if len(dashboard) > 0 {
		rootFS := os.DirFS(dashboard)
		r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			cleanedPath := strings.TrimPrefix(path, "/")
			f, err := rootFS.Open(cleanedPath)
			if err == nil {
				f.Close()
				http.FileServer(http.FS(rootFS)).ServeHTTP(w, r)
				return
			}
			indexPath := filepath.Join(dashboard, "index.html")
			http.ServeFile(w, r, indexPath)
		})
	} else {
		r.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
			writer.WriteHeader(http.StatusOK)
			writer.Write([]byte("pong"))
		})
	}

	r = managementAPI(r, handlers)
	r = publicAPI(r, handlers)

	s.SetHandler(r)
}

func managementAPI(r *chi.Mux, handlers *registry.Handlers) *chi.Mux {
	stopList := admin.NewStopListEndpoint(handlers.Admin.StopList)
	r.Method(http.MethodGet, "/api/admin/v1/stop-list", http.HandlerFunc(stopList.List))
	r.Method(http.MethodPost, "/api/admin/v1/stop-list", http.HandlerFunc(stopList.Create))
	r.Method(http.MethodDelete, "/api/admin/v1/stop-list/{id}", http.HandlerFunc(stopList.Delete))
	r.Method(http.MethodGet, "/api/admin/v1/subscriptions", http.HandlerFunc(stopList.Subscriptions))
	r.Method(http.MethodPut, "/api/admin/v1/subscriptions", http.HandlerFunc(stopList.PutSubscription))
	r.Method(
		http.MethodGet,
		"/api/admin/v1/template",
		admin.NewTemplateFinderEndpoint(handlers.Admin.TemplateFinder))
	r.Method(
		http.MethodPost,
		"/api/admin/v1/template",
		admin.NewTemplateCreationEndpoint(handlers.Admin.TemplateCreator))
	r.Method(
		http.MethodPut,
		"/api/admin/v1/template/{code}",
		admin.NewTemplateUpdateEndpoint(handlers.Admin.TemplateUpdater))

	r.Method(
		http.MethodGet,
		"/api/admin/v1/message",
		admin.NewMessageFinderEndpoint(handlers.Admin.MessageFinder))

	r.Method(
		http.MethodGet,
		"/api/admin/v1/message/{id}",
		admin.NewMessageFinderByIDEndpoint(handlers.Admin.MessageFindByID))

	r.Method(
		http.MethodGet,
		"/api/admin/v1/dictionary/message",
		admin.NewMessageDictionaryEndpoint(handlers.Admin.MessageDictionary))

	r.Method(
		http.MethodGet,
		"/api/admin/v1/dictionary/message-recipients",
		admin.NewMessageRecipientFinderEndpoint(handlers.Admin.MessageRecipientFinder))

	r.Method(
		http.MethodGet,
		"/api/admin/v1/workers/status",
		admin.NewWorkerStatusEndpoint(handlers.Admin.WorkerStatus))

	return r
}

func publicAPI(r *chi.Mux, handlers *registry.Handlers) *chi.Mux {
	r.Method(http.MethodPatch, "/api/v1/templates/{code}/metadata", public.NewTemplateMetadataEndpoint(handlers.Public.TemplateMetadata))
	r.Method(http.MethodGet, "/api/v1/transports", public.NewTransportsEndpoint(handlers.Public.Transports))
	r.HandleFunc("/ping", func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		writer.Write([]byte("pong"))
	})

	r.Method(http.MethodPost,
		"/api/v1/send",
		public.NewSenderEndpoint(handlers.Public.Sender))
	r.Method(http.MethodPost,
		"/api/v1/batch/send",
		public.NewBatchSenderEndpoint(handlers.Public.BatchSender))
	r.Method(http.MethodPost,
		"/api/v1/preview",
		public.NewMessagePreviewEndpoint(handlers.Public.Preview))
	r.Method(http.MethodPost,
		"/api/v1/message/retry",
		public.NewMessageRetryEndpoint(handlers.Public.Retrier))

	return r
}
