import {test} from 'node:test';
import assert from 'node:assert/strict';
import {templateRows, serializeParams} from './params.js';

test('declared parameters require client values and serialize to the API object', () => {
    const rows = templateRows({order_number: {default: 'wrong'}, count: {}}, [{key: 'count', value: 0}]);
    assert.equal(rows[0].value, '');
    assert.equal(rows[0].required, true);
    assert.throws(() => serializeParams(rows));
    rows[0].value = '17829';
    assert.deepEqual(serializeParams(rows), {order_number: {value: '17829'}, count: {value: 0}});
    assert.throws(() => serializeParams([{key: 'a', value: '   '}]));
    assert.throws(() => serializeParams([{key: 'a', value: 1}, {key: 'a', value: 2}]));
    assert.deepEqual(serializeParams([]), {});
    assert.deepEqual(templateRows(null), []);
    assert.deepEqual(serializeParams([{key: 'flag', value: false}]), {flag: {value: false}});
});
