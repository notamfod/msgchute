package template

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/flosch/pongo2/v7"

	"github.com/devian2011/msgchute/internal/dto"
)

var ErrMissingParameters = errors.New("missing required template parameters")

type Generator struct {
	tplSet *pongo2.TemplateSet
}

func NewGenerator() (*Generator, error) {
	tplSet := pongo2.NewSet("generator_set", pongo2.DefaultLoader)

	err := tplSet.RegisterFilter("uppercase", func(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, error) {
		return pongo2.AsValue(strings.ToUpper(in.String())), nil
	})
	if err != nil {
		return nil, err
	}

	return &Generator{
		tplSet: tplSet,
	}, nil
}

func (g *Generator) GenerateString(
	tmpl string,
	messageParams map[string]*dto.MessageParam,
	tmplParams map[string]*dto.TemplateParam,
) (string, error) {
	var missing []string
	for name := range tmplParams {
		param := messageParams[name]
		if param == nil || param.Value == nil {
			missing = append(missing, name)
		} else if value, ok := param.Value.(string); ok && strings.TrimSpace(value) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return "", fmt.Errorf("%w: %s", ErrMissingParameters, strings.Join(missing, ", "))
	}
	pTmpl, err := g.tplSet.FromString(tmpl)
	if err != nil {
		return "", err
	}

	params := g.buildParams(messageParams, tmplParams)
	context := pongo2.Context(params)

	result, err := pTmpl.Execute(context)
	if err != nil {
		return "", err
	}

	return result, nil
}

func (g *Generator) buildParams(
	messageParams map[string]*dto.MessageParam,
	tmplParams map[string]*dto.TemplateParam,
) map[string]interface{} {
	result := make(map[string]interface{}, len(tmplParams))
	for pName, pValue := range tmplParams {
		if inPVal, exists := messageParams[pName]; exists {
			result[pName] = inPVal.Value
		} else {
			result[pName] = pValue.Default
		}
	}
	return result
}
