import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createSubscriptions } from './state.js';

test('new contacts default to order only; changing kind resets channel choices', () => {
  const state = createSubscriptions({});
  assert.deepEqual(state.form.subscriptions, ['email.order']);
  state.form.subscriptions.push('email.news');
  state.reset('phone');
  assert.deepEqual(state.form.subscriptions, ['sms.order']);
});

test('saving an empty explicit list does not restore defaults', async () => {
  let payload;
  const state = createSubscriptions({
    put: async (url, body) => { payload = { url, body }; return { ...body, id: 'saved' }; },
    get: async () => ({ items: [], pagination: { total: 0, total_pages: 0 } }),
  });
  state.form.recipient = ' user@example.com ';
  state.form.subscriptions = [];
  await state.save();
  assert.equal(payload.url, '/api/admin/v1/subscriptions');
  assert.equal(payload.body.recipient, 'user@example.com');
  assert.deepEqual(payload.body.subscriptions, []);
  assert.equal(payload.body.blocked_all, false);
  assert.deepEqual(state.form.subscriptions, []);
});

test('editing and toggling full refusal preserve choices without mutating the list', () => {
  const entry = { id: '1', kind: 'email', recipient: 'user@example.com', subscriptions: ['email.news'], blocked_all: true };
  const state = createSubscriptions({});
  state.edit(entry);
  state.form.blocked_all = false;
  assert.deepEqual(state.form.subscriptions, ['email.news']);
  state.form.subscriptions.push('email.order');
  assert.deepEqual(entry.subscriptions, ['email.news']);
  assert.equal(entry.blocked_all, true);
});

test('save errors preserve user choices; successful save remains visible if refresh fails', async () => {
  const client = { put: async () => { throw new Error('Invalid recipient'); } };
  const state = createSubscriptions(client);
  state.form.recipient = 'invalid';
  state.form.subscriptions = [];
  await state.save();
  assert.equal(state.formError, 'Invalid recipient');
  assert.equal(state.form.recipient, 'invalid');
  assert.deepEqual(state.form.subscriptions, []);
  client.put = async (_, body) => ({ ...body, id: '1' });
  client.get = async () => { throw new Error('Refresh unavailable'); };
  await state.save();
  assert.match(state.notice, /сохранены/);
  assert.equal(state.error, 'Refresh unavailable');
  assert.equal(state.busy, false);
});

test('overlapping save is ignored during loading', async () => {
  let resolve;
  const state = createSubscriptions({ get: () => new Promise(done => { resolve = done; }) });
  const loading = state.load();
  await state.save();
  resolve({ items: [], pagination: { total: 0, total_pages: 0 } });
  await loading;
  assert.equal(state.busy, false);
});
