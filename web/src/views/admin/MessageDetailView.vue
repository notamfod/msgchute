<template>
  <div class="w-100 text-start">
    <div class="d-flex justify-content-between align-items-center mb-4">
      <BButton to="/admin/messages" variant="outline-secondary" size="sm" class="d-flex align-items-center gap-2">
        ← Back to History
      </BButton>
      <BButton variant="primary" size="sm" :disabled="isLoading" @click="fetchSingleRecord">
        Force Refresh
      </BButton>
    </div>

    <div v-if="isLoading" class="text-center py-5">
      <BSpinner variant="primary" label="Loading audit logs..."></BSpinner>
      <div class="text-muted small mt-2">Streaming deep message tracking buffers...</div>
    </div>

    <BAlert v-else-if="error" variant="danger" :model-value="true" class="shadow-sm">
      {{ error }}
    </BAlert>

    <div v-else-if="record" class="animate-fade-in">

      <div class="card shadow-sm border-0 rounded-3 mb-4 bg-dark text-white overflow-hidden">
        <div class="p-4 d-flex flex-wrap justify-content-between align-items-center gap-3">
          <div>
            <span class="text-muted small font-monospace text-uppercase tracking-wider d-block mb-1">Message Identifier Record</span>
            <h1 class="fs-4 fw-bold font-monospace m-0 text-truncate text-warning-glow" style="max-width: 100%;">
              {{ record.message.id }}
            </h1>
          </div>
          <div class="d-flex flex-column align-items-md-end gap-1">
          <span class="badge text-uppercase px-3 py-2 fs-6 tracking-wide"
                :class="getStatusBadgeClass(record.message.status)">
            {{ record.message.status }}
          </span>
            <small class="text-muted-light font-monospace mt-1">Provider: {{ record.message.transport }}</small>
          </div>
        </div>
      </div>

      <BRow class="g-4">
        <BCol lg="4" class="d-flex flex-column gap-4">
          <BCard title="Structural Context" class="shadow-sm border-0 rounded-3">
          <div class="list-group list-group-flush small mt-2">
            <div class="list-group-item px-0 py-2">
              <span class="text-muted d-block small text-uppercase fw-semibold">Sender Context:</span>
              <span class="fw-bold font-monospace text-dark">{{ record.message.senderId }}</span>
            </div>
            <div class="list-group-item px-0 py-2">
              <span class="text-muted d-block small text-uppercase fw-semibold">Тег сообщения:</span>
              <span class="text-dark">{{ record.message.tag || 'Без тега' }}</span>
            </div>
            <div class="list-group-item px-0 py-2">
              <span class="text-muted d-block small text-uppercase fw-semibold">Template Link Code:</span>
              <span class="font-monospace text-secondary fw-semibold">{{ record.message.code || 'Raw Payload (Direct Body)' }}</span>
            </div>
            <div class="list-group-item px-0 py-2">
              <span class="text-muted d-block small text-uppercase fw-semibold">Scheduled Dispatch Window:</span>
              <span class="text-dark">{{ record.message.schedule ? record.message.schedule.toLocaleString() : 'Immediate Action Triggered' }}</span>
            </div>
            <div class="list-group-item px-0 py-2">
              <span class="text-muted d-block small text-uppercase fw-semibold">Hard Delivery Deadline:</span>
              <span class="text-danger font-monospace fw-bold">{{ record.message.deadline ? record.message.deadline.toLocaleString() : 'Infinite Execution Window' }}</span>
            </div>
            <div class="list-group-item px-0 py-2 border-0">
              <span class="text-muted d-block small text-uppercase fw-semibold">Active Tasks Pipeline:</span>
              <span class="badge bg-secondary rounded-pill font-monospace fs-7 mt-1">{{ record.tasks.length }} Tasks Mapped</span>
            </div>
          </div>
          </BCard>
          <BCard title="Template Parameters (Params)" class="shadow-sm border-0 rounded-3">
            <div class="mb-2 text-muted small">
              Dynamic argument parameters variables merged into the compiler layout template runtime engine:
            </div>
            <div v-if="!record.message.params || Object.keys(record.message.params).length === 0" class="p-2 bg-light rounded text-muted fst-italic small border">
              No template parameter mutations provided.
            </div>
            <pre v-else class="bg-light border rounded p-2 m-0 font-monospace raw-inspector-box shadow-inner">{{ JSON.stringify(record.message.params, null, 2) }}</pre>
          </BCard>
          <BCard title="Message Metadata (Meta)" class="shadow-sm border-0 rounded-3">
            <div class="mb-2 text-muted small">
              Additional transport execution context flags (e.g., CC, BCC, routing configurations, file attachments pointers):
            </div>
            <div v-if="!record.message.meta || Object.keys(record.message.meta).length === 0" class="p-2 bg-light rounded text-muted fst-italic small border">
              No additional envelope metadata properties.
            </div>
            <pre v-else class="bg-light border rounded p-2 m-0 font-monospace raw-inspector-box shadow-inner">{{ JSON.stringify(record.message.meta, null, 2) }}</pre>
          </BCard>
          <BCard title="Global Retry Envelope Strategy" class="shadow-sm border-0 rounded-3">
            <div class="mb-2 text-muted small">
              Top-level fallbacks and scheduling adjustments passed down directly by the task producer middleware layer:
            </div>
            <div v-if="!record.message.retry" class="p-2 bg-light rounded text-muted fst-italic small border">
              Using engine default microservice failure retry thresholds.
            </div>
            <pre v-else class="bg-light border rounded p-2 m-0 font-monospace raw-inspector-box shadow-inner">{{ JSON.stringify(record.message.retry, null, 2) }}</pre>
          </BCard>
          <BCard title="Target Envelopes (Recipients)" class="shadow-sm border-0 rounded-3">
            <span class="text-muted d-block small text-uppercase fw-semibold mb-2">Resolved Delivery Routing Addresses:</span>
            <pre class="bg-light border rounded p-2 m-0 font-monospace raw-inspector-box shadow-inner">{{ JSON.stringify(record.message.recipients, null, 2) }}</pre>
          </BCard>
        </BCol>
        <BCol lg="8" class="d-flex flex-column gap-4">
          <BCard title="Message Content Profile" class="shadow-sm border-0 rounded-3">
            <div class="mb-3 mt-2">
              <label class="small text-muted fw-bold text-uppercase d-block mb-1">Subject Header Line:</label>
              <div class="p-2 bg-light border rounded fw-semibold text-dark">{{ record.message.subject }}</div>
            </div>
            <div>
              <label class="small text-muted fw-bold text-uppercase d-block mb-1">Compiled Body Buffer Data:</label>
              <pre class="p-3 bg-dark text-light border-0 rounded font-monospace m-0 body-canvas shadow">{{
                  record.message.body || 'Empty Body Content Streams Detected.'
                }}</pre>
            </div>
          </BCard>

          <BCard title="Asynchronous Task Worker Pipelines" class="shadow-sm border-0 rounded-3">
            <div v-if="record.tasks.length === 0" class="alert alert-info small my-2">
              No engine execution logs recorded against this pipeline envelope context.
            </div>

            <div v-for="fullTask in record.tasks" :key="fullTask.task.id"
                 class="border rounded p-3 mb-3 bg-light-subtle shadow-sm">

              <div
                  class="d-flex flex-wrap justify-content-between align-items-center bg-light p-2 rounded mb-3 border-start border-primary border-3">
                <div>
                  <span class="badge bg-primary text-uppercase px-2 me-2">Worker: {{ fullTask.task.worker }}</span>
                  <code class="text-dark small font-monospace">{{ fullTask.task.id }}</code>
                </div>
                <div class="d-flex align-items-center gap-2">
                <span class="badge"
                      :class="fullTask.task.isProcessed ? 'bg-success-subtle text-success' : 'bg-warning-subtle text-warning'">
                  {{ fullTask.task.isProcessed ? '● Settled' : '○ Pending Queue' }}
                </span>
                  <span class="badge" :class="getStatusBadgeClass(fullTask.task.status)">{{
                      fullTask.task.status
                    }}</span>
                </div>
              </div>

              <BRow class="mb-3 g-2 text-center text-md-start">
                <BCol md="4" class="border-end-md border-light">
                  <span class="small text-muted text-uppercase fw-bold d-block">Retries Progression</span>
                  <div class="fs-4 font-monospace fw-bold text-dark mt-1">
                    {{ fullTask.task.retries }} <span class="text-muted fs-6">/ {{
                      fullTask.task.maxRetries
                    }} max</span>
                  </div>
                </BCol>
                <BCol md="8" class="ps-md-3">
                  <span class="small text-muted text-uppercase fw-bold d-block">Backoff Exponential Delay Profile</span>
                  <div class="small mt-1 text-dark-emphasis font-monospace">
                    <div><strong>Algorithm Rule:</strong> {{ fullTask.task.backOffCode || 'N/A' }}</div>
                    <div class="text-muted text-truncate"><strong>Params:</strong>
                      {{ JSON.stringify(fullTask.task.backOffParams) }}
                    </div>
                  </div>
                </BCol>
              </BRow>

              <div class="mt-4">
                <span class="small text-dark fw-bold text-uppercase d-block mb-3 tracking-wider">✦ Microservice Execution Sequence History</span>
                <div class="timeline-trail position-relative ms-2">

                  <div v-for="(res, index) in fullTask.results" :key="res.id"
                       class="timeline-block position-relative ps-4 pb-3 border-start border-2"
                       :class="res.status === 'Success' ? 'border-success' : 'border-danger'">
                    <span class="position-absolute rounded-circle timeline-dot shadow-sm"
                          :class="res.status === 'Success' ? 'bg-success' : 'bg-danger'"></span>

                    <div class="d-flex justify-content-between align-items-center flex-wrap gap-2 small">
                      <div>
                        <span class="fw-bold me-2" :class="res.status === 'Success' ? 'text-success' : 'text-danger'">Attempt #{{
                            index + 1
                          }} &mdash; {{ res.status }}</span>
                        <span v-if="res.isCritical"
                              class="badge bg-danger text-white py-0 small">CRITICAL FAILURE</span>
                      </div>
                      <span class="text-muted font-monospace">
                      {{ res.runAt ? res.runAt.toLocaleString() : 'N/A' }}
                      <strong class="text-dark ms-1">({{ res.executionTimeMs.toFixed(1) }} ms)</strong>
                    </span>
                    </div>

                    <div class="mt-2">
                      <code
                          class="d-block text-dark bg-white border border-light rounded p-2 font-monospace single-output shadow-inner">
                        {{ res.result || 'Empty result tracer payload string mapped from processing cluster unit.' }}
                      </code>
                    </div>
                  </div>

                </div>
              </div>

            </div>
          </BCard>
        </BCol>
      </BRow>
    </div>
  </div>
</template>

<script setup>
import {onMounted, ref} from 'vue'
import {useRoute} from 'vue-router'
import {messageService} from '@/api/message/message.service'

const route = useRoute()
const record = ref(null)
const isLoading = ref(false)
const error = ref(null)

async function fetchSingleRecord() {
  isLoading.value = true
  error.value = null
  try {
    const messageUuid = route.params.id;
    record.value = await messageService.getMessageById(messageUuid)
  } catch (err) {
    error.value = err.message || 'Unable to extract message transaction profile data aggregates.'
  } finally {
    isLoading.value = false
  }
}

function getStatusBadgeClass(status) {
  if (status === 'succeeded') return 'bg-success'
  if (status === 'pending') return 'bg-warning text-dark'
  return 'bg-danger'
}

onMounted(() => {
  fetchSingleRecord()
})
</script>


<style scoped>
.text-muted-light {
  color: rgba(255, 255, 255, 0.55);
}

.fs-7 {
  font-size: 0.8rem;
}

.text-warning-glow {
  color: #ffc107;
  text-shadow: 0 0 10px rgba(255, 193, 7, 0.15);
}

.raw-inspector-box {
  max-height: 250px;
  overflow-y: auto;
  font-size: 0.75rem;
  background-color: #fafafa;
}

.body-canvas {
  max-height: 350px;
  overflow-y: auto;
  font-size: 0.825rem;
  white-space: pre-wrap;
  word-break: break-all;
}

.single-output {
  font-size: 0.75rem;
  white-space: pre-wrap;
  word-break: break-all;
  background-color: #fcfcfc !important;
}

.timeline-dot {
  width: 12px;
  height: 12px;
  left: -7px;
  top: 4px;
}

@media (min-width: 768px) {
  .border-end-md {
    border-right: 1px solid var(--bs-border-color) !important;
  }
}

.animate-fade-in {
  animation: fadeIn 0.25s ease-out forwards;
}

@keyframes fadeIn {
  from {
    opacity: 0;
    transform: translateY(5px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
</style>
