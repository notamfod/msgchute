<script setup>
import { computed, onMounted } from 'vue';
import apiClient from '@/api/client';
import { createSubscriptions, tagLabels, channelLabels, subscriptionLabel } from '@/api/stop-list/state';

const state = createSubscriptions(apiClient);
const channels = computed(() => state.form.kind === 'email' ? ['email'] : ['sms', 'whatsapp']);
onMounted(() => state.load());
function search() { state.page = 1; state.load(); }
function changePage(page) { state.page = page; state.load(); }
function edit(entry) {
  state.edit(entry);
  document.getElementById('subscriptions-form').scrollIntoView({ behavior: 'smooth', block: 'start' });
}
</script>

<template>
  <section class="card shadow-sm text-start" :aria-busy="state.busy">
    <div class="card-body p-4">
      <h1 class="h3">Подписки получателей</h1>
      <p class="text-secondary">Настройки привязаны к email или телефону. По умолчанию разрешены уведомления по заказу через Email и SMS. Акции и новости требуют подписки.</p>
      <p class="small text-secondary">Сообщения без тега проверяются только на полный отказ. Снятие галочки запрещает соответствующий тип сообщений с тегом.</p>
      <p v-if="state.notice" class="alert alert-success" role="status">{{ state.notice }}</p>
      <div v-if="state.error" class="alert alert-danger" role="alert">
        {{ state.error }}
        <button class="btn btn-outline-danger btn-sm ms-2" :disabled="state.busy" @click="state.load">Обновить список</button>
      </div>

      <form id="subscriptions-form" class="border rounded p-3 mb-4" @submit.prevent="state.save">
        <div class="d-flex flex-wrap justify-content-between gap-2 mb-3">
          <h2 class="h5 mb-0">{{ state.editing ? 'Настройки получателя' : 'Добавить получателя' }}</h2>
          <button v-if="state.editing" type="button" class="btn btn-outline-secondary btn-sm" :disabled="state.busy" @click="state.reset()">Другой получатель</button>
        </div>
        <fieldset :disabled="state.busy">
          <div class="row g-3">
            <div class="col-md-3">
              <label for="subscription-kind" class="form-label">Тип адреса</label>
              <select id="subscription-kind" v-model="state.form.kind" class="form-select" :disabled="state.editing" @change="state.reset(state.form.kind)">
                <option value="email">Email</option><option value="phone">Телефон</option>
              </select>
            </div>
            <div class="col-md-9">
              <label for="subscription-recipient" class="form-label">{{ state.form.kind === 'email' ? 'Email' : 'Телефон с кодом страны' }}</label>
              <input id="subscription-recipient" v-model="state.form.recipient" class="form-control" :readonly="state.editing"
                :type="state.form.kind === 'email' ? 'email' : 'tel'" required maxlength="254"
                :placeholder="state.form.kind === 'email' ? 'user@example.com' : '+7 999 123-45-67'"
                aria-describedby="subscription-error" :aria-invalid="!!state.formError" @input="state.formError = ''">
            </div>
          </div>
          <div class="form-check my-3">
            <input id="blocked-all" v-model="state.form.blocked_all" type="checkbox" class="form-check-input">
            <label for="blocked-all" class="form-check-label fw-semibold">Отказ от всех уведомлений</label>
          </div>
          <p v-if="state.form.blocked_all" class="text-danger small">Отправка на этот адрес запрещена. Выбранные подписки сохранятся после снятия полного отказа.</p>
          <fieldset :disabled="state.form.blocked_all" class="mb-3">
            <legend class="h6">Подписки</legend>
            <div v-for="channel in channels" :key="channel" class="mb-3">
              <div class="fw-semibold mb-2">{{ channelLabels[channel] }}</div>
              <div v-for="(label, tag) in tagLabels" :key="tag" class="form-check">
                <input :id="`${channel}-${tag}`" v-model="state.form.subscriptions" type="checkbox" class="form-check-input" :value="`${channel}.${tag}`">
                <label :for="`${channel}-${tag}`" class="form-check-label">{{ channelLabels[channel] }}. {{ label }}</label>
              </div>
            </div>
          </fieldset>
          <label for="subscription-reason" class="form-label">Комментарий (необязательно)</label>
          <textarea id="subscription-reason" v-model="state.form.reason" class="form-control" rows="2" maxlength="1000" />
          <p id="subscription-error" class="text-danger mt-2 mb-0" role="alert">{{ state.formError }}</p>
          <button type="submit" class="btn btn-primary mt-3">Сохранить подписки</button>
        </fieldset>
      </form>

      <form class="d-flex flex-wrap gap-2 align-items-end mb-3" @submit.prevent="search">
        <div class="flex-grow-1">
          <label for="subscription-search" class="form-label">Поиск получателя</label>
          <input id="subscription-search" v-model="state.search" class="form-control" type="search" maxlength="254" :disabled="state.busy">
        </div>
        <button class="btn btn-outline-primary" :disabled="state.busy">Найти</button>
      </form>
      <p v-if="state.busy" role="status">Загрузка…</p>
      <template v-else-if="!state.error">
        <p class="text-secondary">Настроено адресов: {{ state.pagination.total }}</p>
        <p v-if="!state.items.length">Записи не найдены. Для адресов без записи действуют настройки по умолчанию.</p>
        <div v-else class="table-responsive">
          <table class="table table-striped align-middle">
            <thead><tr><th scope="col">Получатель</th><th scope="col">Подписки</th><th scope="col">Полный отказ</th><th scope="col">Комментарий</th><th scope="col">Действия</th></tr></thead>
            <tbody>
              <tr v-for="entry in state.items" :key="entry.id">
                <td class="text-break">{{ entry.recipient }}</td>
                <td><ul v-if="entry.subscriptions.length" class="mb-0 ps-3"><li v-for="value in entry.subscriptions" :key="value">{{ subscriptionLabel(value) }}</li></ul><span v-else>Нет выбранных типов</span></td>
                <td :class="entry.blocked_all ? 'text-danger fw-semibold' : ''">{{ entry.blocked_all ? 'Да' : 'Нет' }}</td>
                <td class="text-break reason-cell">{{ entry.reason || 'Не указан' }}</td>
                <td><button class="btn btn-outline-primary btn-sm" :aria-label="`Изменить подписки ${entry.recipient}`" @click="edit(entry)">Изменить</button></td>
              </tr>
            </tbody>
          </table>
        </div>
        <nav v-if="state.pagination.total_pages > 1" aria-label="Страницы подписок" class="d-flex gap-3 align-items-center">
          <button class="btn btn-outline-secondary" :disabled="state.page <= 1" @click="changePage(state.page - 1)">Назад</button>
          <span>Страница {{ state.page }} из {{ state.pagination.total_pages }}</span>
          <button class="btn btn-outline-secondary" :disabled="state.page >= state.pagination.total_pages" @click="changePage(state.page + 1)">Далее</button>
        </nav>
      </template>
    </div>
  </section>
</template>

<style scoped>
.reason-cell { max-width: 24rem; white-space: pre-wrap; }
</style>
