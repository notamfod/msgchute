export class TaskExecutionResultDto {
    constructor(data) {
        this.id = data.id;
        this.taskId = data.task_id;
        this.status = data.status;
        this.runAt = data.run_at ? new Date(data.run_at) : null;
        this.result = new TextDecoder().decode(
            Uint8Array.from(atob(data.result), c => c.charCodeAt(0))
        );
        this.isCritical = data.is_critical ?? false;
        this.executionTimeMs = data.execution_time ? data.execution_time / 1000000 : 0;
    }
}

export class TaskDto {
    constructor(data) {
        this.id = data.id;
        this.messageId = data.message_id;
        this.worker = data.worker;
        this.status = data.status;
        this.retries = data.retries ?? 0;
        this.maxRetries = data.max_retries ?? 0;
        this.backOffCode = data.backoff_code;
        this.backOffParams = data.backoff_params || {};
        this.deadline = data.deadline ? new Date(data.deadline) : null;
        this.isProcessed = data.is_processed ?? false;
        this.lockUntil = data.lock_until ? new Date(data.lock_until) : null;
        this.createdAt = data.created_at ? new Date(data.created_at) : null;
        this.lastRun = data.last_run ? new Date(data.last_run) : null;
        this.nextRun = data.next_run ? new Date(data.next_run) : null;
    }
}

export class FullTaskDto {
    constructor(data) {
        this.task = new TaskDto(data.task || {});
        this.results = Array.isArray(data.results)
            ? data.results.map(r => new TaskExecutionResultDto(r))
            : [];
    }
}

export class MessageDto {
    constructor(data) {
        this.id = data.id;
        this.senderId = data.sender_id;
        this.recipients = data.recipients || [];
        this.status = data.status;
        this.meta = data.metadata || {};
        this.code = data.code || null;
        this.params = data.params || {};
        this.transport = data.transport;
        this.tag = data.tag || '';
        this.subject = data.subject || 'No Subject';
        this.body = data.body || '';
        this.deadline = data.deadline ? new Date(data.deadline) : null;
        this.retry = data.retry || null;
        this.schedule = data.schedule ? new Date(data.schedule) : null;
    }
}

export class FullMessageInfoDto {
    constructor(data) {
        this.message = new MessageDto(data.message || {});
        this.tasks = Array.isArray(data.tasks)
            ? data.tasks.map(t => new FullTaskDto(t))
            : [];
    }
}

export class MessageFinderResponseDto {
    constructor(backendData) {
        const rawData = backendData || {};

        this.messages = Array.isArray(rawData.messages)
            ? rawData.messages.map(m => new FullMessageInfoDto(m))
            : [];

        const pag = rawData.pagination || {};
        this.pagination = {
            currentPage: Number(pag.current_page ?? 1),
            perPage: Number(pag.per_page ?? 10),
            total: Number(pag.total ?? 0),
            totalPages: Number(pag.total_pages ?? 1)
        };
    }
}

export class MessageDictionariesDto {
    constructor(backendData) {
        const data = backendData || {};
        this.senderIds = Array.isArray(data.sender_ids) ? data.sender_ids : [];
        this.transports = Array.isArray(data.transports) ? data.transports : [];
        this.templates = Array.isArray(data.templates) ? data.templates : [];
    }
}

export class MessageSendResponse {
    constructor(backendData) {
        this.message = new MessageDto(backendData.message);
    }
}
