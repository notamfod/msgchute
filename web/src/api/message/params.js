export function templateRows(params = {}, previous = []) {
    const values = new Map(previous.map(row => [row.key, row.value]));
    return Object.entries(params ?? {}).map(([key, param]) => ({
        key, value: values.get(key) ?? param?.default ?? '', required: param?.required !== false, declared: true,
    }));
}

export function serializeParams(rows) {
    const entries = rows.map(({key, value, required}) => {
        const blank = value == null || (typeof value === 'string' && !value.trim());
        if (!key.trim() || (required !== false && blank)) {
            throw new Error('Fill every parameter name and value.');
        }
        return [key, {value}];
    });
    if (new Set(entries.map(([key]) => key)).size !== entries.length) {
        throw new Error('Parameter names must be unique.');
    }
    return Object.fromEntries(entries);
}
