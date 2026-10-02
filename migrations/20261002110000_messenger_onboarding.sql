-- migrate:up
CREATE TABLE message_onboarding_invitations
(
    id            UUID PRIMARY KEY,
    task_id       UUID                     NOT NULL UNIQUE REFERENCES tasks (id) ON DELETE CASCADE,
    message_id    UUID                     NOT NULL REFERENCES messages (id) ON DELETE CASCADE,
    transport     VARCHAR(255)             NOT NULL,
    bot_id        VARCHAR(255)             NOT NULL,
    phone         VARCHAR(32)              NOT NULL,
    sms_transport VARCHAR(255)             NOT NULL,
    connect_url   TEXT                     NOT NULL,
    sent_at       TIMESTAMP WITH TIME ZONE          DEFAULT NULL,
    finished_at   TIMESTAMP WITH TIME ZONE          DEFAULT NULL,
    created_at    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX message_onboarding_invitations_active_phone
    ON message_onboarding_invitations (transport, bot_id, phone)
    WHERE finished_at IS NULL;

CREATE TABLE message_onboarding_deliveries
(
    task_id            UUID PRIMARY KEY REFERENCES tasks (id) ON DELETE CASCADE,
    message_id         UUID                     NOT NULL REFERENCES messages (id) ON DELETE CASCADE,
    transport          VARCHAR(255)             NOT NULL,
    bot_id             VARCHAR(255)             NOT NULL,
    recipient          TEXT                     NOT NULL,
    phone              VARCHAR(32)                       DEFAULT NULL,
    selected_path      VARCHAR(32)              NOT NULL DEFAULT 'pending',
    resolved_recipient TEXT                              DEFAULT NULL,
    invitation_id      UUID                              DEFAULT NULL REFERENCES message_onboarding_invitations (id),
    wait_until         TIMESTAMP WITH TIME ZONE          DEFAULT NULL,
    selected_at        TIMESTAMP WITH TIME ZONE          DEFAULT NULL,
    completed_at       TIMESTAMP WITH TIME ZONE          DEFAULT NULL,
    created_at         TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT message_onboarding_deliveries_path CHECK
        (selected_path IN ('pending', 'legacy', 'messenger', 'sms_fallback', 'expired'))
);

CREATE INDEX message_onboarding_deliveries_message_id
    ON message_onboarding_deliveries (message_id);
CREATE INDEX message_onboarding_deliveries_invitation_id
    ON message_onboarding_deliveries (invitation_id);

-- migrate:down
DROP INDEX IF EXISTS message_onboarding_deliveries_invitation_id;
DROP INDEX IF EXISTS message_onboarding_deliveries_message_id;
DROP TABLE IF EXISTS message_onboarding_deliveries;
DROP INDEX IF EXISTS message_onboarding_invitations_active_phone;
DROP TABLE IF EXISTS message_onboarding_invitations;
