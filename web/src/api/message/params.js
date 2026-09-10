export function templateRows(params = {}, previous = []) {
    const values = new Map(previous.map(row => [row.key, row.value]));
    return Object.keys(params ?? {}).map(key => ({key, value: values.get(key) ?? '', required: true}));
}

export function serializeParams(rows) {
    const entries = rows.map(({key, value}) => {
        if (!key.trim() || value == null || (typeof value === 'string' && !value.trim())) {
            throw new Error('Fill every parameter name and value.');
        }
        return [key, {value}];
    });
    if (new Set(entries.map(([key]) => key)).size !== entries.length) {
        throw new Error('Parameter names must be unique.');
    }
    return Object.fromEntries(entries);
}
