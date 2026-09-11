import {test} from 'node:test';
import assert from 'node:assert/strict';
import {templateRows, serializeParams} from './params.js';

test('declared parameters prefill defaults and serialize to the API object', () => {
    const rows = templateRows({order_number: {default: 'wrong'}, count: {}}, [{key: 'count', value: 0}]);
    assert.equal(rows[0].value, 'wrong');
    assert.equal(rows[0].required, false);
    assert.equal(rows[0].declared, true);
    rows[0].value = '17829';
    assert.deepEqual(serializeParams(rows), {order_number: {value: '17829'}, count: {value: 0}});
    assert.throws(() => serializeParams([{key: 'a', value: '   '}]));
    assert.throws(() => serializeParams([{key: 'a', value: 1}, {key: 'a', value: 2}]));
    assert.deepEqual(serializeParams([]), {});
    assert.deepEqual(templateRows(null), []);
    assert.deepEqual(serializeParams([{key: 'flag', value: false}]), {flag: {value: false}});
});

test('clearing an optional default preserves the explicit blank while required blanks fail', () => {
    const rows = templateRows({optional: {required: false, default: 'fallback'}, required: {required: true}});
    assert.equal(rows[0].required, false);
    rows[0].value = '  ';
    rows[1].value = 'present';
    assert.deepEqual(serializeParams(rows), {optional: {value: '  '}, required: {value: 'present'}});
    rows[0].value = '';
    assert.deepEqual(serializeParams(rows), {optional: {value: ''}, required: {value: 'present'}});
    rows[1].value = '';
    assert.throws(() => serializeParams(rows));
});

test('legacy parameters allow blank defaults and omitted requirements', () => {
    const rows = templateRows({email: {default: ' '}, name: {}, other: null});
    assert.ok(rows.every(row => row.required === false));
    assert.deepEqual(serializeParams(rows), {
        email: {value: ' '}, name: {value: ''}, other: {value: ''},
    });
});
