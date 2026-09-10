package template

import (
	"errors"
	"testing"

	"github.com/devian2011/msgchute/internal/dto"
)

func TestGenerator_GenerateString(t *testing.T) {
	type args struct {
		tmpl          string
		messageParams map[string]*dto.MessageParam
		tmplParams    map[string]*dto.TemplateParam
	}

	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "Success with message params",
			args: args{
				tmpl: "Hello, {{ name }}! Your balance: {{ balance }}.",
				messageParams: map[string]*dto.MessageParam{
					"name":    {Value: "John"},
					"balance": {Value: 100},
				},
				tmplParams: map[string]*dto.TemplateParam{
					"name":    {Default: "Guest"},
					"balance": {Default: "0"},
				},
			},
			want:    "Hello, John! Your balance: 100.",
			wantErr: false,
		},
		{
			name: "Missing parameter uses default",
			args: args{
				tmpl:          "Hello, {{ name }}!",
				messageParams: map[string]*dto.MessageParam{},
				tmplParams: map[string]*dto.TemplateParam{
					"name": {Default: "Guest"},
				},
			},
			want:    "Hello, Guest!",
			wantErr: false,
		},
		{
			name: "Success with uppercase filter",
			args: args{
				tmpl: "Hello, {{ name | uppercase }}!",
				messageParams: map[string]*dto.MessageParam{
					"name": {Value: "john"},
				},
				tmplParams: map[string]*dto.TemplateParam{
					"name": {Default: "guest"},
				},
			},
			want:    "Hello, JOHN!",
			wantErr: false,
		},
		{
			name: "Template syntax error",
			args: args{
				tmpl:          "Hello, {{ name",
				messageParams: map[string]*dto.MessageParam{},
				tmplParams:    map[string]*dto.TemplateParam{},
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "Template execution error with invalid filter",
			args: args{
				tmpl:          "Hello, {{ name | undefined_filter }}",
				messageParams: map[string]*dto.MessageParam{"name": {Value: "John"}},
				tmplParams:    map[string]*dto.TemplateParam{"name": {Default: ""}},
			},
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, err := NewGenerator()
			if err != nil {
				t.Fatalf("NewGenerator() returned error: %v", err)
			}

			got, err := g.GenerateString(tt.args.tmpl, tt.args.messageParams, tt.args.tmplParams)
			if (err != nil) != tt.wantErr {
				t.Errorf("Generator.GenerateString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Generator.GenerateString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenerator_buildParams(t *testing.T) {
	g := &Generator{}

	tmplParams := map[string]*dto.TemplateParam{
		"user": {Default: "Anonymous"},
		"age":  {Default: "not specified"},
	}
	messageParams := map[string]*dto.MessageParam{
		"user": {Value: "Alex"},
	}

	res := g.buildParams(messageParams, tmplParams)

	if res["user"] != "Alex" {
		t.Errorf("Expected 'Alex', got: %v", res["user"])
	}
	if res["age"] != "not specified" {
		t.Errorf("Expected 'not specified', got: %v", res["age"])
	}
}

func TestRequiredParameterValues(t *testing.T) {
	for _, tc := range []struct {
		name  string
		param *dto.MessageParam
		valid bool
	}{
		{"absent", nil, true},
		{"null", &dto.MessageParam{}, true},
		{"blank", &dto.MessageParam{Value: " \t"}, false},
		{"zero", &dto.MessageParam{Value: 0}, true},
		{"false", &dto.MessageParam{Value: false}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, err := NewGenerator()
			if err != nil {
				t.Fatal(err)
			}
			_, err = g.GenerateString("{{ value }}", map[string]*dto.MessageParam{"value": tc.param}, map[string]*dto.TemplateParam{"value": {Default: "fallback"}})
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, error=%v", tc.valid, err)
			}
		})
	}
}

func TestParameterRequirements(t *testing.T) {
	optional := false
	required := true
	for _, tc := range []struct {
		name       string
		definition *dto.TemplateParam
		param      *dto.MessageParam
		want       string
		missing    bool
	}{
		{"legacy missing", &dto.TemplateParam{}, nil, "", true},
		{"explicit required missing", &dto.TemplateParam{Required: &required}, nil, "", true},
		{"optional missing", &dto.TemplateParam{Required: &optional}, nil, "", false},
		{"optional blank", &dto.TemplateParam{Required: &optional, Default: "fallback"}, &dto.MessageParam{Value: ""}, "", false},
		{"optional fallback", &dto.TemplateParam{Required: &optional, Default: "fallback"}, nil, "fallback", false},
		{"nil definition missing", nil, nil, "", true},
		{"nil definition supplied", nil, &dto.MessageParam{Value: "supplied"}, "supplied", false},
		{"blank default", &dto.TemplateParam{Default: " \t"}, nil, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, err := NewGenerator()
			if err != nil {
				t.Fatal(err)
			}
			got, err := g.GenerateString("{{ value }}", map[string]*dto.MessageParam{"value": tc.param}, map[string]*dto.TemplateParam{"value": tc.definition})
			if errors.Is(err, ErrMissingParameters) != tc.missing || (!tc.missing && err != nil) || got != tc.want {
				t.Fatalf("got %q, error %v; want %q, missing %v", got, err, tc.want, tc.missing)
			}
		})
	}
}

func BenchmarkGenerator_buildParams(b *testing.B) {
	g := &Generator{}
	tmplParams := map[string]*dto.TemplateParam{
		"name":    {Default: "Guest"},
		"balance": {Default: "0"},
		"status":  {Default: "active"},
	}
	messageParams := map[string]*dto.MessageParam{
		"name":    {Value: "John Doe"},
		"balance": {Value: 250},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = g.buildParams(messageParams, tmplParams)
	}
}

func BenchmarkGenerator_GenerateString_Simple(b *testing.B) {
	g, err := NewGenerator()
	if err != nil {
		b.Fatalf("NewGenerator() returned error: %v", err)
	}
	tmpl := "Hello, {{ name }}! Your balance is {{ balance }}."
	tmplParams := map[string]*dto.TemplateParam{
		"name":    {Default: "Guest"},
		"balance": {Default: "0"},
	}
	messageParams := map[string]*dto.MessageParam{
		"name":    {Value: "John"},
		"balance": {Value: 100},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = g.GenerateString(tmpl, messageParams, tmplParams)
	}
}

func BenchmarkGenerator_GenerateString_WithFilter(b *testing.B) {
	g, err := NewGenerator()
	if err != nil {
		b.Fatalf("NewGenerator() returned error: %v", err)
	}
	tmpl := "Hello, {{ name | uppercase }}!"
	tmplParams := map[string]*dto.TemplateParam{
		"name": {Default: "guest"},
	}
	messageParams := map[string]*dto.MessageParam{
		"name": {Value: "john"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = g.GenerateString(tmpl, messageParams, tmplParams)
	}
}
