package template

import (
	"context"
	"fmt"
	"regexp"

	"github.com/jmoiron/sqlx"

	"github.com/devian2011/msgchute/internal/dto"
	"github.com/devian2011/msgchute/internal/io/storage"
)

var (
	templateTagPattern        = regexp.MustCompile(`(?s){{.*?}}|{%.*?%}`)
	templateIdentifierPattern = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)
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
	if t.BodySource == dto.MessageBodySourceProvided {
		bBody = t.Body
		tmplParams = templateParamsUsedBy(bSubject, tmplParams)
	}

	subject, genSubjectErr = m.generator.GenerateString(bSubject, t.Params, tmplParams)
	if genSubjectErr != nil {
		return "", "", genSubjectErr
	}

	if t.BodySource == dto.MessageBodySourceProvided {
		return subject, bBody, nil
	}

	body, genBodyErr = m.generator.GenerateString(bBody, t.Params, tmplParams)
	if genBodyErr != nil {
		return "", "", genBodyErr
	}

	return subject, body, nil
}

func templateParamsUsedBy(tmpl string, params map[string]*dto.TemplateParam) map[string]*dto.TemplateParam {
	usedNames := make(map[string]struct{})
	for _, tag := range templateTagPattern.FindAllString(tmpl, -1) {
		for _, name := range templateIdentifierPattern.FindAllString(stripQuotedTemplateText(tag), -1) {
			usedNames[name] = struct{}{}
		}
	}
	used := make(map[string]*dto.TemplateParam)
	for name, definition := range params {
		if _, ok := usedNames[name]; ok {
			used[name] = definition
		}
	}
	return used
}

func stripQuotedTemplateText(value string) string {
	result := []rune(value)
	var quote rune
	escaped := false
	for i, char := range result {
		if quote == 0 {
			if char == '\'' || char == '"' {
				quote = char
				result[i] = ' '
			}
			continue
		}
		result[i] = ' '
		if char == quote && !escaped {
			quote = 0
		}
		escaped = char == '\\' && !escaped
		if char != '\\' {
			escaped = false
		}
	}
	return string(result)
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
		// Older clients do not send required when replacing parameter definitions.
		for name, param := range tmpl.Params {
			previous := existing.Params[name]
			if previous == nil || previous.Required == nil || (param != nil && param.Required != nil) {
				continue
			}
			if param == nil {
				param = &dto.TemplateParam{}
				tmpl.Params[name] = param
			}
			param.Required = previous.Required
		}

		return m.repo.Update(ctx, tmpl)
	})

	if trxErr != nil {
		return nil, trxErr
	}
	return tmpl, nil
}
