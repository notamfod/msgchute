import { reactive } from 'vue';

const path = '/api/admin/v1/subscriptions';
const newForm = (kind = 'email') => ({
  kind, recipient: '', reason: '', blocked_all: false,
  subscriptions: [kind === 'email' ? 'email.order' : 'sms.order'],
});

export const tagLabels = { order: 'Уведомления по заказу', promotion: 'Акции и предложения', news: 'Новости' };
export const channelLabels = { email: 'Email', sms: 'SMS', whatsapp: 'WhatsApp' };
export function subscriptionLabel(value) {
  const [channel, tag] = value.split('.');
  return `${channelLabels[channel] || channel}: ${tagLabels[tag] || tag}`;
}

export function createSubscriptions(client) {
  const state = reactive({
    items: [], page: 1, search: '', pagination: { total: 0, total_pages: 0 },
    busy: false, error: '', formError: '', notice: '', editing: false,
    form: newForm(),
    reset(kind = 'email') {
      state.form = newForm(kind);
      state.editing = false;
      state.formError = '';
      state.notice = '';
    },
    edit(entry) {
      state.form = {
        kind: entry.kind, recipient: entry.recipient, reason: entry.reason || '',
        blocked_all: entry.blocked_all, subscriptions: [...entry.subscriptions],
      };
      state.editing = true;
      state.formError = '';
      state.notice = '';
    },
    async load() {
      if (state.busy) return;
      state.busy = true;
      state.error = '';
      try {
        const result = await client.get(path, {
          params: { page: state.page, per_page: 20, search: state.search.trim() },
        });
        state.items = result.items;
        state.pagination = result.pagination;
      } catch (error) {
        state.error = error.message;
      } finally {
        state.busy = false;
      }
    },
    async save() {
      if (state.busy) return;
      state.busy = true;
      state.formError = '';
      state.notice = '';
      try {
        const saved = await client.put(path, {
          kind: state.form.kind, recipient: state.form.recipient.trim(),
          reason: state.form.reason.trim(), blocked_all: state.form.blocked_all,
          subscriptions: [...state.form.subscriptions],
        });
        state.edit(saved);
        state.notice = 'Настройки подписок сохранены.';
        state.page = 1;
      } catch (error) {
        state.formError = error.message;
        return;
      } finally {
        state.busy = false;
      }
      await state.load();
    },
  });
  return state;
}
