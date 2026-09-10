package template

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/devian2011/msgchute/internal/dto"
	"github.com/devian2011/msgchute/internal/io/storage"
)

type generator interface {
	GenerateString(
		tmpl string,
		msgParams map[string]*dto.MessageParam,
		tmplParams map[string]*dto.TemplateParam,
	) (string, error)
}

type templateRepo interface {
	GetByCode(ctx context.Context, code string) (*dto.Template, error)
	Find(ctx context.Context, filter *dto.MessageTemplateFilter) (map[string]*dto.Template, uint64, error)
	Create(ctx context.Context, t *dto.Template) error
	Update(ctx context.Context, t *dto.Template) error
	UpdateMetadata(ctx context.Context, code string, metadata dto.TemplateMetadata) error
}

type Manager struct {
	db        *sqlx.DB
	generator generator
	repo      templateRepo
}

func NewManager(db *sqlx.DB, generator generator, repo templateRepo) *Manager {
	return &Manager{
		db:        db,
		generator: generator,
		repo:      repo,
	}
}

func (m *Manager) GenerateMessage(t *dto.Message) (subject string, body string, err error) {
	var (
		tmplParams = make(map[string]*dto.TemplateParam)

		bSubject = t.Subject
		bBody    = t.Body

		genSubjectErr error
		genBodyErr    error
	)

	if t.Code != nil && len(*t.Code) > 0 {
		tmpl, tmplGetErr := m.repo.GetByCode(context.Background(), *t.Code)
		if tmplGetErr != nil {
			return "", "", fmt.Errorf("get template by code err: %w", tmplGetErr)
		}
		if tmpl == nil {
			return "", "", fmt.Errorf("template with code: %s not exists", *t.Code)
		}
		tmplParams = tmpl.Params
		bSubject = tmpl.Subject
		bBody = tmpl.Body
	}

	subject, genSubjectErr = m.generator.GenerateString(bSubject, t.Params, tmplParams)
	if genSubjectErr != nil {
		return "", "", genSubjectErr
	}

	body, genBodyErr = m.generator.GenerateString(bBody, t.Params, tmplParams)
	if genBodyErr != nil {
		return "", "", genBodyErr
	}

	return subject, body, nil
}

func (m *Manager) Find(filter *dto.MessageTemplateFilter) (map[string]*dto.Template, uint64, error) {
	return m.repo.Find(context.Background(), filter)
}

func (m *Manager) Create(tmpl *dto.Template) (*dto.Template, error) {
	trxErr := storage.InTransaction(context.Background(), m.db, func(ctx context.Context) error {
		existing, getErr := m.repo.GetByCode(ctx, tmpl.Code)
		if getErr != nil {
			return fmt.Errorf("get template by code err: %w", getErr)
		}
		if existing != nil {
			return fmt.Errorf("template with code: %s already exists", tmpl.Code)
		}

		return m.repo.Create(ctx, tmpl)
	})

	if trxErr != nil {
		return nil, trxErr
	}
	return tmpl, nil
}

func (m *Manager) Update(tmpl *dto.Template) (*dto.Template, error) {
	trxErr := storage.InTransaction(context.Background(), m.db, func(ctx context.Context) error {
		existing, getErr := m.repo.GetByCode(ctx, tmpl.Code)
		if getErr != nil {
			return fmt.Errorf("get template by code err: %w", getErr)
		}
		if existing == nil {
			return fmt.Errorf("template with code: %s not found", tmpl.Code)
		}
		tmpl.Metadata = existing.Metadata
		if tmpl.Systems == nil {
			tmpl.Systems = existing.Systems
		}
		if tmpl.Channels == nil {
			tmpl.Channels = existing.Channels
		}

		return m.repo.Update(ctx, tmpl)
	})

	if trxErr != nil {
		return nil, trxErr
	}
	return tmpl, nil
}
