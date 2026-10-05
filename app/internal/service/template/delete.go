package template

import (
	"context"

	"github.com/devian2011/msgchute/internal/io/storage"
)

func (m *Manager) Delete(ctx context.Context, code string) error {
	return storage.InTransaction(ctx, m.db, func(txCtx context.Context) error {
		tmpl, err := m.repo.GetByCode(txCtx, code)
		if err != nil {
			return err
		}
		if tmpl == nil {
			return ErrTemplateNotFound
		}
		return m.repo.Delete(txCtx, code)
	})
}
