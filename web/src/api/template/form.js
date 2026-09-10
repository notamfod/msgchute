export function splitLabels(value) {
    return [...new Set(value.split(',').map(label => label.trim()).filter(Boolean))];
}

export function templatePayload(form) {
    const {code, name, description, subject, body, params} = form;
    return {code, name, description, subject, body, params,
        systems: splitLabels(form.systemsRaw), channels: splitLabels(form.channelsRaw)};
}
