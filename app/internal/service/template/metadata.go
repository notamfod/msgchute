package template

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/devian2011/msgchute/internal/dto"
	"github.com/devian2011/msgchute/internal/io/storage"
)

var ErrTemplateNotFound = errors.New("template not found")
var ErrMetadataTooLarge = errors.New("template metadata exceeds 64 KiB")

const MetadataLimit = 64 * 1024

// Merge objects recursively; null deletes a key and arrays/scalars replace it.
func mergeMetadata(current, patch map[string]any) {
	for key, value := range patch {
		if value == nil {
			delete(current, key)
			continue
		}
		if object, ok := value.(map[string]any); ok {
			existing, ok := current[key].(map[string]any)
			if !ok {
				existing = map[string]any{}
			}
			mergeMetadata(existing, object)
			current[key] = existing
		} else {
			current[key] = value
		}
	}
}

func (m *Manager) PatchMetadata(ctx context.Context, code string, patch dto.TemplateMetadata) (dto.TemplateMetadata, error) {
	var result dto.TemplateMetadata
	err := storage.InTransaction(ctx, m.db, func(txCtx context.Context) error {
		tmpl, err := m.repo.GetByCode(txCtx, code)
		if err != nil {
			return err
		}
		if tmpl == nil {
			return ErrTemplateNotFound
		}
		result = tmpl.Metadata
		if result == nil {
			result = dto.TemplateMetadata{}
		}
		mergeMetadata(result, patch)
		data, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if len(data) > MetadataLimit {
			return ErrMetadataTooLarge
		}
		return m.repo.UpdateMetadata(txCtx, code, result)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
