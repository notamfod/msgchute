<template>
  <BCard title="Message Templates Management" class="shadow border-0 rounded-3 text-start">

    <div class="d-flex justify-content-between align-items-center mb-4">
      <h5 class="m-0 text-secondary">Manage system alert layout records</h5>
      <BButton variant="success" @click="openCreateModal">
        + Create New Template
      </BButton>
    </div>

    <div v-if="loading" class="text-center py-4">
      <BSpinner variant="primary"/>
    </div>

    <BAlert v-else-if="error" variant="danger" :model-value="true">
      {{ error }}
    </BAlert>

    <div v-else-if="templatesMap">
      <BCard class="mb-4 bg-light border-0">
        <BForm @submit.prevent="forceSearch">
          <BRow class="g-3 align-items-center">
            <BCol cols="12" md="4">
              <BFormInput
                  v-model="filters.search"
                  type="text"
                  placeholder="Search by name or content..."
              />
            </BCol>

            <BCol cols="12" md="4">
              <BFormInput
                  v-model="codeInputField"
                  type="text"
                  placeholder="Filter by codes (comma separated)..."
                  @change="handleCodeFilterChange"
              />
            </BCol>

            <BCol cols="12" md="4">
              <BFormGroup label="Systems (comma separated)">
                <BFormInput v-model="systemInputField" />
              </BFormGroup>
            </BCol>
            <BCol cols="12" md="4">
              <BFormGroup label="Channels (comma separated)">
                <BFormInput v-model="channelInputField" />
              </BFormGroup>
            </BCol>
            <BCol cols="12" md="4" class="d-flex gap-2">
              <BButton type="submit" variant="primary" class="w-100">
                Search
              </BButton>
              <BButton type="button" variant="outline-secondary" class="w-100" @click="clearFilters">
                Reset
              </BButton>
            </BCol>
          </BRow>
        </BForm>
      </BCard>

      <div class="d-flex align-items-center justify-content-end mb-3 px-2 gap-3">
        <div v-if="filters.sortField" class="text-muted xs font-monospace bg-light px-2 py-0.5 rounded border">
          Sorted by: <span class="text-primary fw-bold">{{ filters.sortField }}</span> ({{ filters.sortOrder }})
        </div>

        <div class="text-secondary small fw-medium d-flex align-items-center gap-2">
          <svg xmlns="http://w3.org" width="14" height="14" fill="currentColor" class="text-primary" viewBox="0 0 16 16">
            <path fill-rule="evenodd" d="M2 2.5a.5.5 0 0 0-.5.5v1a.5.5 0 0 0 .5.5h1a.5.5 0 0 0 .5-.5V3a.5.5 0 0 0-.5-.5zM3 3H2v1h1zM5 3.5a.5.5 0 0 1 .5-.5h9a.5.5 0 0 1 0 1h-9a.5.5 0 0 1-.5-.5zM5.5 7a.5.5 0 0 0 0 1h9a.5.5 0 0 0 0-1zM5.5 11a.5.5 0 0 0 0 1h9a.5.5 0 0 0 0-1zM2 7a.5.5 0 0 0-.5.5v1a.5.5 0 0 0 .5.5h1a.5.5 0 0 0 .5-.5v-1a.5.5 0 0 0-.5-.5zm1 .5H2v1h1zm-1 3.5a.5.5 0 0 0-.5.5v1a.5.5 0 0 0 .5.5h1a.5.5 0 0 0 .5-.5v-1a.5.5 0 0 0-.5-.5zm1 .5H2v1h1z"/>
          </svg>
          <span>
            Showing <B class="text-dark fw-bold">{{ Object.keys(templatesMap).length }}</B>
            of <B class="text-dark fw-bold">{{ pagination.total || 0 }}</B> total templates found
          </span>
        </div>
      </div>

      <BTableSimple striped hover responsive bordered class="align-middle mb-0">
        <BThead head-variant="dark">
          <BTr>
            <BTh style="width: 25%; cursor: pointer;" @click="toggleSort('code')">
              Code
              <span v-if="filters.sortField === 'code'">{{ filters.sortOrder === 'asc' ? '▲' : '▼' }}</span>
            </BTh>
            <BTh style="width: 25%; cursor: pointer;" @click="toggleSort('name')">
              Name
              <span v-if="filters.sortField === 'name'">{{ filters.sortOrder === 'asc' ? '▲' : '▼' }}</span>
            </BTh>
            <BTh style="width: 35%">Description</BTh>
            <BTh style="width: 15%" class="text-end">Actions</BTh>
          </BTr>
        </BThead>

        <BTbody>
          <template v-for="(template, code) in templatesMap" :key="code">
            <BTr>
              <BTd><code class="text-danger font-monospace fw-bold">{{ template.code }}</code></BTd>
              <BTd class="fw-semibold">{{ template.name }}</BTd>
              <BTd class="text-muted text-truncate" style="max-width: 300px;">
                {{ template.description || '—' }}
              </BTd>
              <BTd class="text-end">
                <BButton
                    :variant="editingCode === template.code ? 'secondary' : 'primary'"
                    size="sm"
                    @click="toggleEditForm(template)"
                >
                  {{ editingCode === template.code ? 'Close' : 'Edit' }}
                </BButton>
              </BTd>
            </BTr>
            <BTr v-if="editingCode === template.code" class="table-light">
              <BTd colspan="4" class="p-4">
                <BCard border-variant="primary" header-bg-variant="primary" header-text-variant="white">
                  <template #header>
                    <h5 class="mb-0">Edit Template: {{ editForm.code }}</h5>
                  </template>

                  <BForm @submit.prevent="saveTemplate">
                    <BRow class="g-3">
                      <BCol cols="12">
                        <BFormGroup label="Template Name *" label-class="fw-bold">
                          <BFormInput v-model="editForm.name" required/>
                        </BFormGroup>
                      </BCol>

                      <BCol cols="12">
                        <BFormGroup label="Description" label-class="fw-bold">
                          <BFormTextarea v-model="editForm.description" rows="2"/>
                        </BFormGroup>
                      </BCol>

                      <BCol md="6">
                        <BFormGroup label="Systems (comma separated)">
                          <BFormInput v-model="editForm.systemsRaw" />
                        </BFormGroup>
                      </BCol>
                      <BCol md="6">
                        <BFormGroup label="Channels (comma separated)">
                          <BFormInput v-model="editForm.channelsRaw" />
                        </BFormGroup>
                      </BCol>
                      <!-- TEMPLATE PARAMS INLINE CONFIGURATION -->
                      <BCol cols="12">
                        <span
                            class="fw-bold d-block mb-2">Template Injection Parameters (Object Configuration Map)</span>

                        <!-- Inline field addition layout input -->
                        <div class="d-flex gap-2 mb-3">
                          <BFormInput
                              v-model="newParamKeys.edit"
                              placeholder="Type new parameter key (e.g., user_id)..."
                              size="sm"
                              @keydown.enter.prevent="addNewParam('edit')"
                          />
                          <BButton size="sm" variant="outline-primary" @click="addNewParam('edit')">
                            Add Field
                          </BButton>
                        </div>

                        <BCard class="bg-white border mb-2" v-if="Object.keys(editForm.params).length > 0">
                          <BRow class="g-2">
                            <BCol cols="12" v-for="(paramObj, paramKey) in editForm.params" :key="paramKey"
                                  class="border-bottom pb-2 mb-2">
                              <BRow class="align-items-end g-2">
                                <BCol cols="4">
                                  <BFormGroup label="Param Key Token String"
                                              label-class="small text-muted font-monospace">
                                    <BFormInput :model-value="paramKey" readonly disabled
                                                class="bg-light font-monospace fw-bold text-dark"/>
                                  </BFormGroup>
                                </BCol>
                                <BCol cols="6">
                                  <BFormGroup label="Default value (optional)"
                                              label-class="small text-muted font-monospace">
                                    <BFormInput v-model="paramObj.default" placeholder="Default value" />
                                    <BFormCheckbox v-model="paramObj.required">Required</BFormCheckbox>
                                  </BFormGroup>
                                </BCol>
                                <BCol cols="2" class="text-end">
                                  <BButton variant="outline-danger" size="sm" @click="removeParam('edit', paramKey)">
                                    Remove
                                  </BButton>
                                </BCol>
                              </BRow>
                            </BCol>
                          </BRow>
                        </BCard>
                        <div v-else class="text-muted small border rounded p-3 bg-white text-center">
                          No parameters specified yet. Variables will be transmitted clean.
                        </div>
                      </BCol>

                      <BCol cols="12">
                        <BFormGroup label="Subject *" label-class="fw-bold">
                          <BFormInput v-model="editForm.subject" required/>
                        </BFormGroup>
                      </BCol>

                      <BCol cols="12">
                        <BFormGroup label="Body *" label-class="fw-bold">
                          <BFormTextarea v-model="editForm.body" rows="6" required/>
                        </BFormGroup>
                      </BCol>
                    </BRow>

                    <div class="mt-4 d-flex gap-2">
                      <BButton type="submit" variant="success" :disabled="saving">
                        <BSpinner v-if="saving" small class="me-1"></BSpinner>
                        Save Changes
                      </BButton>
                      <BButton variant="outline-secondary" @click="editingCode = null">
                        Cancel
                      </BButton>
                    </div>
                  </BForm>
                </BCard>
              </BTd>
            </BTr>
          </template>
        </BTbody>
      </BTableSimple>

      <BAlert
          v-if="Object.keys(templatesMap).length === 0"
          :model-value="true"
          variant="warning"
          class="text-center mt-3"
      >
        No templates found matching current criteria.
      </BAlert>

      <div v-if="pagination.total_pages > 1" class="d-flex justify-content-center mt-4">
        <BPagination
            v-model="filters.page"
            :total-rows="pagination.total_items || (pagination.total_pages * filters.per_page)"
            :per-page="filters.per_page"
            @update:model-value="changePage"
        />
      </div>
    </div>

    <BModal
        v-slot="{ hide }"
        v-model="createModalOpen"
        title="Create Message Template"
        size="lg"
        no-close-on-backdrop
        ok-title="Create"
        cancel-title="Close"
        ok-variant="success"
        cancel-variant="outline-secondary"
        :ok-disabled="saving"
        @ok.prevent="createTemplate"
    >
      <BForm @submit.prevent="createTemplate">
        <BRow class="g-3">
          <BCol cols="12">
            <BFormGroup label="Template Code" label-class="fw-bold">
              <BFormInput v-model="createForm.code" placeholder="e.g., SMTP_TRANSACTION_RECEIPT" required/>
            </BFormGroup>
          </BCol>

          <BCol cols="12">
            <BFormGroup label="Template Display Name *" label-class="fw-bold">
              <BFormInput v-model="createForm.name" placeholder="User-friendly layout name identifier" required />
            </BFormGroup>
          </BCol>

          <BCol cols="12">
            <BFormGroup label="Description" label-class="fw-bold">
              <BFormTextarea v-model="createForm.description" rows="2" placeholder="Describe deployment contexts" />
            </BFormGroup>
          </BCol>

          <BCol cols="12">
            <BFormGroup label="Systems (comma separated)" class="mb-3">
              <BFormInput v-model="createForm.systemsRaw" />
            </BFormGroup>
            <BFormGroup label="Channels (comma separated)" class="mb-3">
              <BFormInput v-model="createForm.channelsRaw" />
            </BFormGroup>
            <span class="fw-bold d-block mb-2">Template Map Keys Initialization</span>

            <div class="d-flex gap-2 mb-3">
              <BFormInput
                  v-model="newParamKeys.create"
                  placeholder="Type new parameter key..."
                  size="sm"
                  @keydown.enter.prevent="addNewParam('create')"
              />
              <BButton size="sm" variant="outline-primary" @click="addNewParam('create')">
                Add Parameter
              </BButton>
            </div>

            <BCard class="bg-light border mb-2" v-if="Object.keys(createForm.params).length > 0">
              <div v-for="(pObj, pKey) in createForm.params" :key="pKey" class="bg-white p-2 rounded mb-2 border">
                <BRow class="align-items-end g-2">
                  <BCol cols="4">
                    <BFormGroup label="Property Field Key" label-class="small text-muted font-monospace">
                      <BFormInput :model-value="pKey" readonly disabled class="bg-light text-dark font-monospace fw-bold" />
                    </BFormGroup>
                  </BCol>
                  <BCol cols="6">
                    <BFormGroup label="Default value (optional)" label-class="small text-muted font-monospace">
                      <BFormInput v-model="pObj.default" placeholder="Default value" />
                      <BFormCheckbox v-model="pObj.required">Required</BFormCheckbox>
                    </BFormGroup>
                  </BCol>
                  <BCol cols="2" class="text-end">
                    <BButton variant="outline-danger" size="sm" @click="removeParam('create', pKey)">
                      Remove
                    </BButton>
                  </BCol>
                </BRow>
              </div>
            </BCard>
          </BCol>

          <BCol cols="12">
            <BFormGroup label="Subject Title * (Notification Subject Line Layout)" label-class="fw-bold">
              <BFormInput v-model="createForm.subject" placeholder="Welcome aboard, {{ name }}!" required />
            </BFormGroup>
          </BCol>

          <BCol md="12" class="mb-2">
            <div class="d-flex justify-content-between align-items-center mb-1">
              <label class="small fw-semibold text-muted m-0">Message Body Content:</label>

              <div class="d-flex align-items-center gap-2">
                <span class="small text-muted font-monospace">Mode:</span>
                <select v-model="editorMode" class="form-select form-select-sm font-monospace py-0 px-2" style="width: auto; height: 24px; font-size: 0.75rem;">
                  <option value="html">HTML</option>
                  <option value="json">JSON</option>
                  <option value="yaml">YAML</option>
                </select>
              </div>
            </div>

            <!-- Окно редактора CodeMirror -->
            <div class="code-editor-wrapper rounded overflow-hidden border shadow-inner">
              <Codemirror
                  v-model="createForm.body"
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
        </BRow>
      </BForm>
    </BModal>

  </BCard>
</template>

<script setup>
import {computed, onMounted, reactive, ref} from 'vue';
import {templateService} from '@/api/template/template.service.js';
import {splitLabels, templatePayload} from '@/api/template/form.js';
import {Codemirror} from "vue-codemirror";
import {json} from "@codemirror/lang-json";
import {yaml} from "@codemirror/lang-yaml";
import {html} from "@codemirror/lang-html";
import {BCard} from "bootstrap-vue-next";

const templatesMap = ref({});
const pagination = ref({total_pages: 0, current_page: 1, total: 0});
const loading = ref(false);
const saving = ref(false);
const error = ref(null);
const codeInputField = ref('');
const systemInputField = ref('');
const channelInputField = ref('');

const filters = reactive({
  page: 1,
  per_page: 10,
  search: '',
  code: [],
  system: [],
  channel: [],
  sortField: '',
  sortOrder: 'asc'
});

const editingCode = ref(null);
const editForm = reactive({
  systemsRaw: '',
  channelsRaw: '',
  code: '',
  name: '',
  description: '',
  subject: '',
  body: '',
  params: {}
});

const createModalOpen = ref(false);
const createForm = reactive({
  systemsRaw: '',
  channelsRaw: '',
  code: '',
  name: '',
  description: '',
  subject: '',
  body: '',
  params: {}
});

// Holds current text input buffer strings for new parameter fields
const newParamKeys = reactive({
  edit: '',
  create: ''
});

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
  if (editorMode.value !== 'json' || !createForm.body.trim()) return false
  try {
    JSON.parse(createForm.body)
    return false
  } catch {
    return true
  }
})

let debounceTimeout = null;

const fetchTemplates = async () => {
  loading.value = true;
  error.value = null;
  try {
    const {page, per_page, sortField, sortOrder, ...extraFilters} = filters;
    const requestFilters = {
      ...extraFilters,
      sort: sortField || undefined,
      order: sortOrder || undefined
    };
    const data = await templateService.getTemplates(page, per_page, requestFilters);
    templatesMap.value = data.templates || {};
    pagination.value = data.pagination || {total_pages: 1, total: 0};
  } catch (err) {
    error.value = err.message || 'Failed to sync application layouts directories.';
  } finally {
    loading.value = false;
  }
};

const toggleSort = (targetField) => {
  if (filters.sortField === targetField) {
    filters.sortOrder = filters.sortOrder === 'asc' ? 'desc' : 'asc';
  } else {
    filters.sortField = targetField;
    filters.sortOrder = 'asc';
  }
  filters.page = 1;
  fetchTemplates();
};

const toggleEditForm = (template) => {
  if (editingCode.value === template.code) {
    editingCode.value = null;
  } else {
    editingCode.value = template.code;
    editForm.code = template.code;
    editForm.name = template.name;
    editForm.description = template.description;
    editForm.subject = template.subject;
    editForm.body = template.body;
    editForm.systemsRaw = (template.systems || []).join(', ');
    editForm.channelsRaw = (template.channels || []).join(', ');
    newParamKeys.edit = ''; // clear any remaining input text

    const rawParams = template.params || {};
    const formattedParams = {};
    Object.keys(rawParams).forEach(k => {
      formattedParams[k] = {
        required: rawParams[k]?.required !== false,
        default: typeof rawParams[k] === 'object' && rawParams[k] !== null
            ? (rawParams[k].default || '')
            : String(rawParams[k] ?? '')
      };
    });
    editForm.params = formattedParams;
  }
};

// Directly handles input field maps using the reactive string references
const addNewParam = (targetForm) => {
  const rawKey = targetForm === 'edit' ? newParamKeys.edit : newParamKeys.create;
  if (!rawKey) return;

  const normalizedKey = rawKey.trim().replace(/[^a-zA-Z0-9_]/g, '');
  if (!normalizedKey) {
    alert("Invalid identifier format string. Use alphanumerics and underscores only.");
    return;
  }

  if (targetForm === 'edit') {
    if (!editForm.params[normalizedKey]) {
      editForm.params[normalizedKey] = {default: '', required: true};
    }
    newParamKeys.edit = ''; // Clear input buffer string on success
  } else {
    if (!createForm.params[normalizedKey]) {
      createForm.params[normalizedKey] = {default: '', required: true};
    }
    newParamKeys.create = ''; // Clear input buffer string on success
  }
};

const removeParam = (targetForm, targetKey) => {
  if (targetForm === 'edit') {
    delete editForm.params[targetKey];
  } else {
    delete createForm.params[targetKey];
  }
};

const saveTemplate = async () => {
  saving.value = true;
  error.value = null;
  try {
    await templateService.updateTemplate(editForm.code, templatePayload(editForm));
    editingCode.value = null;
    await fetchTemplates();
  } catch (err) {
    error.value = err.message || 'Error processing validation rules.';
  } finally {
    saving.value = false;
  }
};

const openCreateModal = () => {
  createForm.code = '';
  createForm.name = '';
  createForm.description = '';
  createForm.subject = '';
  createForm.body = '';
  createForm.params = {};
  createForm.systemsRaw = '';
  createForm.channelsRaw = '';
  newParamKeys.create = ''; // clear input parameters buffer
  createModalOpen.value = true;
};

const createTemplate = async () => {
  saving.value = true;
  error.value = null;
  try {
    await templateService.createTemplate(createForm.code, templatePayload(createForm));
    createModalOpen.value = false;
    await fetchTemplates();
  } catch (err) {
    error.value = err.message || 'Failed to complete process blueprint context creation.';
  } finally {
    saving.value = false;
  }
};

const parseCodeInputField = () => {
  filters.code = splitLabels(codeInputField.value);
  filters.system = splitLabels(systemInputField.value);
  filters.channel = splitLabels(channelInputField.value);
};

const handleCodeFilterChange = () => {
  parseCodeInputField();
  forceSearch();
};

const forceSearch = () => {
  clearTimeout(debounceTimeout);
  parseCodeInputField();
  filters.page = 1;
  fetchTemplates();
};

const clearFilters = () => {
  filters.search = '';
  filters.code = [];
  filters.sortField = '';
  filters.sortOrder = 'asc';
  codeInputField.value = '';
  systemInputField.value = '';
  channelInputField.value = '';
  forceSearch();
};

const changePage = (page) => {
  filters.page = page;
  fetchTemplates();
};

onMounted(() => {
  fetchTemplates();
});
</script>

<style scoped>
.font-monospace {
  font-family: var(--bs-font-monospace);
}

.table-responsive {
  overflow-y: visible !important;
}

th {
  user-select: none;
  position: relative;
}

th span {
  font-size: 11px;
  margin-left: 4px;
  color: #0d6efd;
}
</style>
