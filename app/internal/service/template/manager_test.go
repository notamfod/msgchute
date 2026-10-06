package template

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/devian2011/msgchute/internal/dto"
	"github.com/devian2011/msgchute/pkg/helper"
)

type MockRepo struct {
	mock.Mock
}

func (m *MockRepo) Delete(ctx context.Context, code string) error {
	return m.Called(ctx, code).Error(0)
}

func (m *MockRepo) UpdateMetadata(ctx context.Context, code string, metadata dto.TemplateMetadata) error {
	return m.Called(ctx, code, metadata).Error(0)
}

func (m *MockRepo) GetByCode(ctx context.Context, code string) (*dto.Template, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.Template), args.Error(1)
}

func (m *MockRepo) Find(ctx context.Context, filter *dto.MessageTemplateFilter) (map[string]*dto.Template, uint64, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Get(1).(uint64), args.Error(2)
	}
	return args.Get(0).(map[string]*dto.Template), args.Get(1).(uint64), args.Error(2)
}

func (m *MockRepo) Create(ctx context.Context, t *dto.Template) error {
	args := m.Called(ctx, t)
	return args.Error(0)
}

func (m *MockRepo) Update(ctx context.Context, t *dto.Template) error {
	args := m.Called(ctx, t)
	return args.Error(0)
}

type MockStringGenerator struct {
	mock.Mock
}

func (m *MockStringGenerator) GenerateString(tmpl string, msgParams map[string]*dto.MessageParam, tmplParams map[string]*dto.TemplateParam) (string, error) {
	args := m.Called(tmpl, msgParams, tmplParams)
	return args.String(0), args.Error(1)
}

func TestManager_GenerateMessage(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		msg           *dto.Message
		mockSetup     func(repo *MockRepo, gen *MockStringGenerator)
		expectedSubj  string
		expectedBody  string
		expectedError bool
	}{
		{
			name: "success without template code",
			msg: &dto.Message{
				Subject: "Hello {{.Name}}",
				Body:    "Body {{.Name}}",
				Params:  dto.MessageParams{"Name": {Value: "John"}},
			},
			mockSetup: func(repo *MockRepo, gen *MockStringGenerator) {
				gen.On("GenerateString", "Hello {{.Name}}", mock.Anything, mock.Anything).
					Return("Hello John", nil).Once()
				gen.On("GenerateString", "Body {{.Name}}", mock.Anything, mock.Anything).
					Return("Body John", nil).Once()
			},
			expectedSubj:  "Hello John",
			expectedBody:  "Body John",
			expectedError: false,
		},
		{
			name: "success with template code",
			msg: &dto.Message{
				Code:    helper.Ptr("welcome"),
				Params:  dto.MessageParams{"Name": {Value: "Alice"}},
				Subject: "ignored",
				Body:    "ignored",
			},
			mockSetup: func(repo *MockRepo, gen *MockStringGenerator) {
				tmpl := &dto.Template{
					Code:    "welcome",
					Params:  dto.TemplateParams{"Title": {Default: "Mr"}},
					Subject: "Template Subject {{.Name}}",
					Body:    "Template Body {{.Name}}",
				}
				repo.On("GetByCode", ctx, "welcome").Return(tmpl, nil).Once()
				gen.On("GenerateString", "Template Subject {{.Name}}", mock.Anything, mock.Anything).
					Return("Template Subject Alice", nil).Once()
				gen.On("GenerateString", "Template Body {{.Name}}", mock.Anything, mock.Anything).
					Return("Template Body Alice", nil).Once()
			},
			expectedSubj:  "Template Subject Alice",
			expectedBody:  "Template Body Alice",
			expectedError: false,
		},
		{
			name: "template not found",
			msg: &dto.Message{
				Code: helper.Ptr("missing"),
			},
			mockSetup: func(repo *MockRepo, gen *MockStringGenerator) {
				repo.On("GetByCode", ctx, "missing").Return(nil, errors.New("not found")).Once()
			},
			expectedError: true,
		},
		{
			name: "generator error on subject",
			msg: &dto.Message{
				Subject: "Hello {{.Name}}",
				Params:  dto.MessageParams{"Name": {Value: "John"}},
			},
			mockSetup: func(repo *MockRepo, gen *MockStringGenerator) {
				gen.On("GenerateString", "Hello {{.Name}}", mock.Anything, mock.Anything).
					Return("", errors.New("gen error")).Once()
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := new(MockRepo)
			gen := new(MockStringGenerator)
			tt.mockSetup(repo, gen)

			db, _, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			sqlxDB := sqlx.NewDb(db, "postgres")
			mgr := NewManager(sqlxDB, gen, repo)

			subj, body, err := mgr.GenerateMessage(tt.msg)
			if tt.expectedError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedSubj, subj)
			assert.Equal(t, tt.expectedBody, body)

			repo.AssertExpectations(t)
			gen.AssertExpectations(t)
		})
	}
}

func TestManager_GenerateMessageWithProvidedBody(t *testing.T) {
	required := true
	for _, tt := range []struct {
		name        string
		message     *dto.Message
		template    *dto.Template
		wantSubject string
		wantBody    string
		wantMissing bool
	}{
		{
			name: "without template code preserves body bytes",
			message: &dto.Message{
				BodySource: dto.MessageBodySourceProvided,
				Subject:    "Plain subject",
				Body:       "Привет\n&<> {{ untouched }} {% untouched %}",
				Params:     dto.MessageParams{"name": {Value: "Alice"}},
			},
			wantSubject: "Plain subject",
			wantBody:    "Привет\n&<> {{ untouched }} {% untouched %}",
		},
		{
			name: "body-only required parameter does not block provided body",
			message: &dto.Message{
				Code:       helper.Ptr("welcome"),
				BodySource: dto.MessageBodySourceProvided,
				Body:       "edited {{ body_value }}",
				Params:     dto.MessageParams{"subject_value": {Value: "Alice"}},
			},
			template: &dto.Template{
				Code:    "welcome",
				Subject: "Hello {{ subject_value }}",
				Body:    "Original {{ body_value }}",
				Params: dto.TemplateParams{
					"subject_value": {Required: &required},
					"body_value":    {Required: &required},
				},
			},
			wantSubject: "Hello Alice",
			wantBody:    "edited {{ body_value }}",
		},
		{
			name: "required subject parameter still enforced",
			message: &dto.Message{
				Code:       helper.Ptr("welcome"),
				BodySource: dto.MessageBodySourceProvided,
				Body:       "edited",
			},
			template: &dto.Template{
				Code:    "welcome",
				Subject: "Hello {{ subject_value }}",
				Body:    "Original {{ body_value }}",
				Params: dto.TemplateParams{
					"subject_value": {Required: &required},
					"body_value":    {Required: &required},
				},
			},
			wantMissing: true,
		},
		{
			name: "template source still renders template body",
			message: &dto.Message{
				Code:       helper.Ptr("welcome"),
				Body:       "ignored",
				BodySource: dto.MessageBodySourceTemplate,
				Params: dto.MessageParams{
					"subject_value": {Value: "Alice"},
					"body_value":    {Value: "rendered"},
				},
			},
			template: &dto.Template{
				Code:    "welcome",
				Subject: "Hello {{ subject_value }}",
				Body:    "Original {{ body_value }}",
				Params: dto.TemplateParams{
					"subject_value": {Required: &required},
					"body_value":    {Required: &required},
				},
			},
			wantSubject: "Hello Alice",
			wantBody:    "Original rendered",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := new(MockRepo)
			if tt.template != nil {
				repo.On("GetByCode", context.Background(), tt.template.Code).Return(tt.template, nil).Once()
			}
			generator, err := NewGenerator()
			require.NoError(t, err)
			db, _, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			manager := NewManager(sqlx.NewDb(db, "postgres"), generator, repo)
			subject, body, err := manager.GenerateMessage(tt.message)
			if tt.wantMissing {
				require.ErrorIs(t, err, ErrMissingParameters)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantSubject, subject)
			require.Equal(t, tt.wantBody, body)
			repo.AssertExpectations(t)
		})
	}
}

func TestTemplateParamsUsedBy(t *testing.T) {
	params := dto.TemplateParams{
		"name":        {},
		"show_name":   {},
		"literal_key": {},
		"body_only":   {},
	}
	used := templateParamsUsedBy(
		`{% if show_name %}{{ name | uppercase }} {{ "literal_key" }}{% endif %}`,
		params,
	)
	require.ElementsMatch(t, []string{"name", "show_name"}, mapKeys(used))
}

func mapKeys(values map[string]*dto.TemplateParam) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func TestManager_Find(t *testing.T) {
	repo := new(MockRepo)
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "postgres")
	mgr := NewManager(sqlxDB, &MockStringGenerator{}, repo)

	filter := &dto.MessageTemplateFilter{Limit: 10}
	expectedResult := map[string]*dto.Template{"code1": {Code: "code1"}}
	expectedTotal := uint64(1)

	repo.On("Find", context.Background(), filter).Return(expectedResult, expectedTotal, nil).Once()

	result, total, err := mgr.Find(filter)
	assert.NoError(t, err)
	assert.Equal(t, expectedResult, result)
	assert.Equal(t, expectedTotal, total)
	repo.AssertExpectations(t)
}

func TestManager_Create(t *testing.T) {
	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "postgres")
	repo := new(MockRepo)
	gen := new(MockStringGenerator)
	mgr := NewManager(sqlxDB, gen, repo)

	tmpl := &dto.Template{Code: "new_template"}

	t.Run("success", func(t *testing.T) {
		sqlMock.ExpectBegin()
		repo.On("GetByCode", mock.Anything, "new_template").Return(nil, nil).Once()
		repo.On("Create", mock.Anything, tmpl).Return(nil).Once()
		sqlMock.ExpectCommit()

		created, err := mgr.Create(tmpl)
		assert.NoError(t, err)
		assert.Equal(t, tmpl, created)
		repo.AssertExpectations(t)
		assert.NoError(t, sqlMock.ExpectationsWereMet())
	})

	t.Run("template already exists", func(t *testing.T) {
		sqlMock.ExpectBegin()
		existing := &dto.Template{Code: "new_template"}
		repo.On("GetByCode", mock.Anything, "new_template").Return(existing, nil).Once()
		sqlMock.ExpectRollback()

		created, err := mgr.Create(tmpl)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
		assert.ErrorIs(t, err, ErrTemplateAlreadyExists)
		assert.Nil(t, created)
		repo.AssertExpectations(t)
		assert.NoError(t, sqlMock.ExpectationsWereMet())
	})

	t.Run("concurrent insert conflicts", func(t *testing.T) {
		sqlMock.ExpectBegin()
		repo.On("GetByCode", mock.Anything, "new_template").Return(nil, nil).Once()
		repo.On("Create", mock.Anything, tmpl).Return(&pgconn.PgError{Code: "23505", ConstraintName: "message_templates_pkey"}).Once()
		sqlMock.ExpectRollback()
		created, err := mgr.Create(tmpl)
		assert.Nil(t, created)
		assert.ErrorIs(t, err, ErrTemplateAlreadyExists)
		repo.AssertExpectations(t)
		assert.NoError(t, sqlMock.ExpectationsWereMet())
	})

	t.Run("get error", func(t *testing.T) {
		sqlMock.ExpectBegin()
		repo.On("GetByCode", mock.Anything, "new_template").Return(nil, errors.New("db error")).Once()
		sqlMock.ExpectRollback()

		created, err := mgr.Create(tmpl)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "get template by code err")
		assert.Nil(t, created)
		repo.AssertExpectations(t)
		assert.NoError(t, sqlMock.ExpectationsWereMet())
	})
}

func TestManager_Update(t *testing.T) {
	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "postgres")
	repo := new(MockRepo)
	gen := new(MockStringGenerator)
	mgr := NewManager(sqlxDB, gen, repo)

	tmpl := &dto.Template{Code: "existing_template", Metadata: dto.TemplateMetadata{"ignored": true}}

	t.Run("success", func(t *testing.T) {
		sqlMock.ExpectBegin()
		existing := &dto.Template{Code: "existing_template", Metadata: dto.TemplateMetadata{"1c": true}, Systems: dto.TemplateLabels{"1c"}, Channels: dto.TemplateLabels{"sms"}}
		repo.On("GetByCode", mock.Anything, "existing_template").Return(existing, nil).Once()
		repo.On("Update", mock.Anything, tmpl).Return(nil).Once()
		sqlMock.ExpectCommit()

		updated, err := mgr.Update(tmpl)
		assert.NoError(t, err)
		assert.Equal(t, tmpl, updated)
		assert.Equal(t, existing.Metadata, updated.Metadata)
		assert.Equal(t, existing.Systems, updated.Systems)
		assert.Equal(t, existing.Channels, updated.Channels)
		repo.AssertExpectations(t)
		assert.NoError(t, sqlMock.ExpectationsWereMet())
	})

	t.Run("template not found", func(t *testing.T) {
		sqlMock.ExpectBegin()
		repo.On("GetByCode", mock.Anything, "existing_template").Return(nil, nil).Once()
		sqlMock.ExpectRollback()

		updated, err := mgr.Update(tmpl)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
		assert.Nil(t, updated)
		repo.AssertExpectations(t)
		assert.NoError(t, sqlMock.ExpectationsWereMet())
	})

	t.Run("update error", func(t *testing.T) {
		sqlMock.ExpectBegin()
		existing := &dto.Template{Code: "existing_template"}
		repo.On("GetByCode", mock.Anything, "existing_template").Return(existing, nil).Once()
		repo.On("Update", mock.Anything, tmpl).Return(errors.New("update failed")).Once()
		sqlMock.ExpectRollback()

		updated, err := mgr.Update(tmpl)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "update failed")
		assert.Nil(t, updated)
		repo.AssertExpectations(t)
		assert.NoError(t, sqlMock.ExpectationsWereMet())
	})
}

func TestManager_UpdatePreservesParameterRequirements(t *testing.T) {
	for _, tc := range []struct {
		name     string
		existing *dto.TemplateParam
		incoming *dto.TemplateParam
		want     *bool
	}{
		{"legacy PUT preserves optional", &dto.TemplateParam{Required: helper.Ptr(false)}, &dto.TemplateParam{Default: "new"}, helper.Ptr(false)},
		{"legacy PUT preserves required", &dto.TemplateParam{Required: helper.Ptr(true)}, &dto.TemplateParam{Default: "new"}, helper.Ptr(true)},
		{"null definition preserves required", &dto.TemplateParam{Required: helper.Ptr(true)}, nil, helper.Ptr(true)},
		{"explicit false overrides true", &dto.TemplateParam{Required: helper.Ptr(true)}, &dto.TemplateParam{Required: helper.Ptr(false)}, helper.Ptr(false)},
		{"explicit true overrides false", &dto.TemplateParam{Required: helper.Ptr(false)}, &dto.TemplateParam{Required: helper.Ptr(true)}, helper.Ptr(true)},
		{"legacy remains legacy", &dto.TemplateParam{}, &dto.TemplateParam{}, nil},
		{"new parameter remains legacy", nil, &dto.TemplateParam{}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, sqlMock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			repo := new(MockRepo)
			mgr := NewManager(sqlx.NewDb(db, "postgres"), &MockStringGenerator{}, repo)
			existing := &dto.Template{Code: "contacts", Params: dto.TemplateParams{
				"email": tc.existing, "removed": {Required: helper.Ptr(true)},
			}}
			incoming := &dto.Template{Code: "contacts", Params: dto.TemplateParams{"email": tc.incoming}}
			sqlMock.ExpectBegin()
			repo.On("GetByCode", mock.Anything, "contacts").Return(existing, nil).Once()
			repo.On("Update", mock.Anything, mock.MatchedBy(func(value *dto.Template) bool {
				assert.NotContains(t, value.Params, "removed")
				require.NotNil(t, value.Params["email"])
				assert.Equal(t, tc.want, value.Params["email"].Required)
				if tc.incoming != nil {
					assert.Equal(t, tc.incoming.Default, value.Params["email"].Default)
				}
				return true
			})).Return(nil).Once()
			sqlMock.ExpectCommit()
			updated, err := mgr.Update(incoming)
			require.NoError(t, err)
			assert.Equal(t, tc.want, updated.Params["email"].Required)
			repo.AssertExpectations(t)
			require.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}
