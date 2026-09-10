<template>
  <div class="w-100 text-start animate-fade-in mb-5">
    <BCard title="Compose Notification Dispatch" class="shadow border-0 rounded-3 p-3">
      <BForm @submit.prevent="submitMessage">
        <BRow class="g-3">

          <!-- 1. IDENTITIES & DELIVERY TARGETS -->
          <BCol md="6">
            <BFormGroup label="Initiator ID (not the SMS or email sender):" class="small fw-semibold text-muted">
              <BFormInput v-model="form.sender_id" type="text" placeholder="e.g., system-billing"
                          class="custom-select2-input" required/>
            </BFormGroup>
          </BCol>

          <BCol md="6">
            <BFormGroup label="Delivery profile:" class="small fw-semibold text-muted">
              <select v-model="form.transport" class="form-select custom-select2-input" required>
                <option value="" disabled>Select Provider...</option>
                <option v-for="t in dictionaries.transports" :key="t.code" :value="t.code" :disabled="!t.available">{{ t.code }} ({{ t.provider }}){{ t.sender ? ': ' + t.sender : '' }}</option>
              </select>
            </BFormGroup>
          </BCol>

          <BCol md="12">
            <BFormGroup label="Recipients (Type values and press Enter):" class="small fw-semibold text-muted">
              <Multiselect
                  v-model="form.recipients"
                  mode="tags"
                  placeholder="Add emails, phone numbers, or webhook channels..."
                  :searchable="true"
                  :create-option="true"
                  :options="[]"
                  required
                  class="custom-select2"
              />
              <small>SMS: international phone number, e.g. +79261234567. Email: user@example.com. Other channels use their provider's recipient format.</small>
            </BFormGroup>
          </BCol>

          <BCol md="12">
            <BFormGroup label="Template Layout Code (Optional Link):" class="small fw-semibold text-muted">
              <select v-model="form.code" class="form-select custom-select2-input" @change="handleTemplateChange">
                <option :value="null">None (Direct Content Mode)</option>
                <option v-for="(tmpl, key) in dictionaries.templates" :key="key" :value="key">{{ key }}</option>
              </select>
            </BFormGroup>
          </BCol>

          <BCol md="12">
            <BFormGroup label="Subject Header Line:" class="small fw-semibold text-muted">
              <BFormInput v-model="form.subject" type="text" placeholder="Type notification subject header..."
                          class="custom-select2-input"/>
            </BFormGroup>
          </BCol>

          <BCol md="12" class="mb-2">
            <div class="d-flex justify-content-between align-items-center mb-1">
              <label class="small fw-semibold text-muted m-0">Message Body Content:</label>

              <div class="d-flex align-items-center gap-2">
                <span class="small text-muted font-monospace">Mode:</span>
                <select v-model="editorMode" class="form-select form-select-sm font-monospace py-0 px-2"
                        style="width: auto; height: 24px; font-size: 0.75rem;">
                  <option value="html">HTML</option>
                  <option value="json">JSON</option>
                  <option value="yaml">YAML</option>
                </select>
              </div>
            </div>

            <!-- Окно редактора CodeMirror -->
            <div class="code-editor-wrapper rounded overflow-hidden border shadow-inner">
              <Codemirror
                  v-model="form.body"
                  :placeholder="editorPlaceholder"
                  :style="{ height: '250px' }"
                  :autofocus="false"
                  :indent-with-tab="true"
                  :tab-size="2"
                  :extensions="currentExtensions"
              />
            </div>
            <div v-if="editorMode === 'json' && bodyJsonError" class="text-danger small mt-1">
              ✕ Invalid JSON structure in message body.
            </div>
          </BCol>

          <BCol md="6">
            <BFormGroup label="Deferred Schedule Window (Optional):" class="small fw-semibold text-muted">
              <BFormInput v-model="form.scheduleRaw" type="datetime-local" class="custom-select2-input"/>
            </BFormGroup>
          </BCol>

          <BCol md="6">
            <BFormGroup label="Hard Eviction Deadline:" class="small fw-semibold text-muted">
              <BFormInput v-model="form.deadlineRaw" type="datetime-local" class="custom-select2-input" required/>
            </BFormGroup>
          </BCol>

          <BCol md="12">
            <BFormGroup label="Raw Custom Metadata Configuration (Meta Object Map):"
                        class="small fw-semibold text-muted">
              <textarea v-model="form.metaRaw" class="form-control font-monospace code-textarea" rows="3"
                        placeholder='{\n  "priority": "high"\n}'></textarea>
              <div v-if="metaJsonError" class="text-danger small mt-1">✕ Invalid JSON structure mapped in custom meta
                attributes.
              </div>
            </BFormGroup>
          </BCol>

          <!-- 4. DYNAMIC MESSAGE PARAMS MANAGED GRID -->
          <BCol cols="12" class="border-top pt-3">
            <div class="d-flex justify-content-between align-items-center mb-2">
              <span class="small fw-bold text-dark text-uppercase tracking-wider">✦ Message Compilation Parameters Mapping</span>
              <BButton type="button" variant="outline-primary" size="sm" @click="addParamRow">+ Add Parameter Key
              </BButton>
            </div>

            <div v-if="form.params.length === 0"
                 class="p-3 bg-light rounded text-center text-muted small border border-dashed">
              No variable substitution bindings compiled against this dispatch yet.
            </div>

            <div v-else class="d-flex flex-column gap-2">
              <div v-for="(row, idx) in form.params" :key="idx" class="d-flex gap-2 align-items-center animate-row">
                <BFormInput v-model="row.key" type="text" placeholder="Key (e.g., first_name)" size="sm"
                            class="font-monospace form-control-sm" style="flex: 1;" :readonly="row.declared" required/>
                <BFormInput v-model="row.value" type="text" placeholder="Value (e.g., John)" size="sm"
                            class="form-control-sm" style="flex: 2;" :required="row.required !== false"/>
                <span class="small text-muted">{{ row.required === false ? 'Optional' : 'Required' }}</span>
                <BButton type="button" variant="outline-danger" size="sm" class="px-2" :disabled="row.declared" @click="removeParamRow(idx)">
                  &times;
                </BButton>
              </div>
            </div>
          </BCol>

          <!-- 5. RETRIER CRITICAL CONFIGURATIONS DECK -->
          <BCol cols="12" class="border-top pt-3">
            <span class="small ref-bold text-dark text-uppercase tracking-wider d-block mb-3">✦ Retrier Policy Pipeline Adjustment Metrics</span>
            <BRow class="g-3 bg-light p-3 rounded border mx-0">

              <BCol md="3">
                <BFormGroup label="Max Retries Bound:" class="small fw-semibold text-secondary">
                  <BFormInput v-model.number="form.retry.retries" type="number" min="0" max="50"
                              class="custom-select2-input bg-white" required/>
                </BFormGroup>
              </BCol>

              <BCol md="4">
                <BFormGroup label="Backoff BackOffCode Strategy:" class="small fw-semibold text-secondary">
                  <select
                      v-model="form.retry.strategy"
                      class="form-select custom-select2-input bg-white"
                      required
                      @change="handleStrategyChange"
                  >
                    <option value="none">none (Immediate Exits)</option>
                    <option v-for="(item, key) in backOffParams" :key="key" :value="key">
                      {{ key }}
                    </option>
                  </select>
                </BFormGroup>
              </BCol>

              <BCol md="5">
                <div class="d-flex justify-content-between align-items-center mb-1">
                  <span class="small fw-semibold text-secondary">Backoff Parameters Map</span>
                </div>

                <div v-if="!form.retry.params || form.retry.params.length === 0 || form.retry.strategy === 'none'"
                     class="text-muted small py-2 fst-italic text-center border bg-white rounded">
                  No tracking intervals registered.
                </div>

                <div v-else class="d-flex flex-column gap-2 dropdown-max-box">
                  <div v-for="(bRow, bIdx) in form.retry.params" :key="bIdx" class="d-flex gap-1 align-items-center">
                    <!-- IMMUTABLE READ-ONLY TEXT FIELD RETAINING VALUE BOUND DATA -->
                    <BFormInput
                        :model-value="bRow.key"
                        type="text"
                        readonly
                        disabled
                        class="bg-light font-monospace form-control-sm text-dark fw-bold border-secondary-subtle"
                        style="flex: 1.2;"
                    />

                    <!-- MUTABLE ACTION ROW VALUES -->
                    <BFormInput
                        v-model="bRow.value"
                        type="text"
                        placeholder="Value (e.g., 2s, 1.5)"
                        size="sm"
                        class="form-control-sm"
                        style="flex: 1.5;"
                        required
                    />

                    <BButton
                        type="button"
                        variant="outline-danger"
                        size="sm"
                        class="py-0 px-2"
                        @click="removeBackoffParamRow(bIdx)"
                    >
                      &times;
                    </BButton>
                  </div>
                </div>
              </BCol>

            </BRow>
          </BCol>

        </BRow>

        <!-- Form Submission Core Actions -->
        <div class="d-flex justify-content-end gap-2 mt-4 border-top pt-3">
          <BButton type="button" variant="outline-secondary" size="sm" @click="resetForm">Clear</BButton>
          <BButton type="submit" variant="primary" size="sm" class="px-5 shadow-sm"
                   :disabled="isSending || metaJsonError">
            <BSpinner v-if="isSending" small class="me-1"></BSpinner>
            Send
          </BButton>
        </div>
      </BForm>
    </BCard>
  </div>
</template>

<script setup>
import {computed, onMounted, reactive, ref} from 'vue'
import {messageService} from '@/api/message/message.service.js'
import {templateRows, serializeParams} from '@/api/message/params.js'
import {templateService} from "@/api/template/template.service.js";
import Multiselect from '@vueform/multiselect'

import {Codemirror} from 'vue-codemirror'
import {html} from '@codemirror/lang-html'
import {json} from '@codemirror/lang-json'
import {yaml} from '@codemirror/lang-yaml'

import '@vueform/multiselect/themes/default.css'
import {backOffParams} from "@/dict/backoff.dict.js";

const isSending = ref(false)

const editorMode = ref('html')

const currentExtensions = computed(() => {
  if (editorMode.value === 'json') return [json()]
  if (editorMode.value === 'yaml') return [yaml()]
  return [html()]
})

const editorPlaceholder = computed(() => {
  if (editorMode.value === 'json') return '{\n  "message": "Hello from JSON payload"\n}'
  if (editorMode.value === 'yaml') return 'message: "Hello from YAML payload"'
  return '<!-- Write your HTML here -->\n<div>Hello World</div>'
})

const bodyJsonError = computed(() => {
  if (editorMode.value !== 'json' || !form.body.trim()) return false
  try {
    JSON.parse(form.body)
    return false
  } catch {
    return true
  }
})

const dictionaries = reactive({
  transports: [],
  templates: []
})

const form = reactive({
  sender_id: '',
  transport: '',
  code: null,
  subject: '',
  body: '',
  recipients: [],
  scheduleRaw: '',
  deadlineRaw: null,
  metaRaw: '{}',
  params: [],
  retry: {
    retries: 3,
    strategy: null,
    params: []
  }
})

// Computed Validation Layer Tracker
const metaJsonError = computed(() => {
  if (!form.metaRaw.trim()) return false
  try {
    JSON.parse(form.metaRaw);
    return false
  } catch {
    return true
  }
})

function addParamRow() {
  form.params.push({key: '', value: ''})
}

function removeParamRow(idx) {
  form.params.splice(idx, 1)
}

function addBackoffParamRow() {
  form.retry.params.push({key: 'min_interval', value: ''})
}

function serializePayload() {
  const mappedBackoffParams = {}
  if (form.retry.strategy !== 'none') {
    form.retry.params.forEach(bItem => {
      const scalarValue = parseFloat(bItem.value)
      mappedBackoffParams[bItem.key] = isNaN(scalarValue) || bItem.value.includes('s')
          ? bItem.value
          : scalarValue
    })
  }

  return {
    sender_id: form.sender_id,
    transport: form.transport,
    code: form.code,
    subject: form.subject,
    body: form.body,
    recipients: form.recipients,
    meta: form.metaRaw.trim() ? JSON.parse(form.metaRaw) : {},
    params: serializeParams(form.params),
    deadline: new Date(form.deadlineRaw).toISOString(),
    schedule: form.scheduleRaw ? new Date(form.scheduleRaw).toISOString() : new Date().toISOString(),
    retry: {
      retries: form.retry.retries,
      strategy: form.retry.strategy,
      params: mappedBackoffParams
    }
  }
}

async function submitMessage() {
  if (metaJsonError.value || bodyJsonError.value) return
  isSending.value = true

  try {
    const finalPayload = serializePayload()
    let result = await messageService.sendNewMessage(finalPayload)
    if (result.message) {
      alert('Message accepted into the queue. Delivery is not yet confirmed. ID: ' + result.message.id);
    }
    resetForm();
  } catch (err) {
    alert('Request error: ' + err.message + '\nIf the request timed out, check message history before sending again. Acceptance may have succeeded.')
  } finally {
    isSending.value = false
  }
}

async function loadDictionaries() {
  try {
    const transports = await messageService.getTransports()
    const data = await templateService.getTemplates(1, 1_000_000);
    if (transports) {
      dictionaries.transports = transports
    }
    if (data) {
      dictionaries.templates = data.templates || []
    }
  } catch (err) {
    console.error('Lookup parameters error context trace logs:', err)
  }
}

const handleTemplateChange = (newKey) => {
  const targetKey = newKey?.target ? newKey.target.value : newKey;
  const templateDetails = dictionaries.templates[targetKey];

  form.params = templateRows(templateDetails?.params, form.params);
};


function resetForm() {
  form.sender_id = ''
  form.transport = ''
  form.code = null
  form.subject = ''
  form.body = ''
  form.recipients = []
  form.scheduleRaw = ''
  form.metaRaw = '{}'
  form.params = []
  form.retry.retries = 3
  form.retry.strategy = 'none'
  form.retry.params = []
}

const computedAvailableOptions = computed(() => {
  const currentStrategy = form.retry.strategy;
  if (currentStrategy && backOffParams[currentStrategy]) {
    return Object.keys(backOffParams[currentStrategy]);
  }
  return [];
});

const handleStrategyChange = () => {
  const selectedStrategy = form.retry.strategy;

  if (selectedStrategy === 'none' || !backOffParams[selectedStrategy]) {
    form.retry.params = [];
    return;
  }

  const defaults = backOffParams[selectedStrategy];

  form.retry.params = Object.entries(defaults).map(([k, v]) => ({
    key: k,
    value: String(v)
  }));
};

const removeBackoffParamRow = (index) => {
  if (form.retry.params && form.retry.params[index]) {
    form.retry.params.splice(index, 1);
  }
};

onMounted(() => {
  loadDictionaries()
})
</script>

<style scoped>

.code-editor-wrapper {
  background-color: #ffffff;
  border-color: #dee2e6 !important;
}

:deep(.cm-editor) {
  font-family: var(--bs-font-monospace), monospace !important;
  font-size: 0.85rem !important;
}

:deep(.cm-scroller) {
  overflow-y: auto;
}

.code-textarea {
  font-size: 0.75rem;
  background-color: #fafafa;
  resize: vertical;
}

.custom-select2-input {
  height: 38px;
  font-size: 0.875rem;
  border-color: #dee2e6;
  border-radius: 0.375rem;
}

.custom-select2 {
  --ms-border-color: #dee2e6;
  --ms-radius: 0.375rem;
  --ms-font-size: 0.875rem;
  --ms-tag-bg: #e9ecef;
  --ms-tag-color: #212529;
  --ms-tag-radius: 0.25rem;
  --ms-border-color-active: #86b7fe;
  --ms-ring-color: rgba(13, 110, 253, 0.25);
  min-height: 38px;
}

.border-dashed {
  border-style: dashed !important;
}

.dropdown-max-box {
  max-height: 120px;
  overflow-y: auto;
}

.animate-row {
  animation: slideIn 0.15s ease-out forwards;
}

@keyframes slideIn {
  from {
    opacity: 0;
    transform: translateY(3px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@media (min-width: 768px) {
  .border-end-md {
    border-right: 1px solid var(--bs-border-color) !important;
  }
}
</style>
