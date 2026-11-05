import { parser } from '../dist/index.es.js';
import { fileTests } from '@lezer/generator/dist/test';

import * as fs from 'fs';
import * as path from 'path';
import { fileURLToPath } from 'url';

let caseDir = path.dirname(fileURLToPath(import.meta.url))
for (const file of fs.readdirSync(caseDir)) {
    if (!/\.txt$/.test(file)) continue;

    const name = /^[^.]*/.exec(file)[0];
    describe(name, () => {
        for (const { name, run } of fileTests(fs.readFileSync(path.join(caseDir, file), 'utf8'), file)) it(name, () => run(parser));
    });
}

// Parse strictly so error recovery cannot hide a backend/frontend mismatch.
describe('unquoted UTF-8 names', () => {
    const strictParser = parser.configure({ strict: true });
    const valid = [
        'http.server.request.duration{service.name="api"}',
        '温度{場所="東京"}',
        '𐐀{𐐀="x"}',
        'a𐐀9{__名="x"}',
        'sum by (service.name, équipe) (rate(http.server.requests[5m]))',
        'sum without (場所) (温度)',
        'requests_total + on (service.name) group_left (équipe) service_info',
        'requests_total + ignoring (service.name) group_right (équipe) service_info',
        'foo{a..="b"}',
        '_:温度.total',
        'foo.',
        'foo..bar',
        'Inf.total + Inf',
        'NaN.total or foo',
        'http.server.requests[5m:1m] offset 5m',
        'http.server.requests @ 0.5',
        'http.server.requests + .5',
        'foo > bool.5',
        'sum by ("a:b", "ö", "😀") (foo)',
        'foo{".name"="x", "ö"="y", "😀"="z"}',
    ];
    for (const query of valid) {
        it(`accepts ${query}`, () => {
            expect(() => strictParser.parse(query)).not.toThrow();
        });
    }
    const invalid = [
        '.foo', '5foo', 'foo{.label="x"}', 'foo{5label="x"}', 'foo{a:b="x"}',
        'foo😀', 'foo١', 'foo²', 'o\u0308', 'foo·bar', 'foo\u200dbar', 'foo\ufffd',
        'foo\ud800', 'foo\udc00', 'sum by (o\u0308) (foo)', 'foo{é😀="x"}',
        'sum by (a:b) (foo)', 'foo + on (a:b) bar', 'topK(1, foo)',
    ];
    for (const query of invalid) {
        it(`rejects ${query}`, () => {
            expect(() => strictParser.parse(query)).toThrow();
        });
    }
    // Only bool, offset and keyword operators end before a dot and a digit, and only
    // when they are the whole leading word.
    for (const name of ['İnf', 'topK', 'İGNORING', 'on.total', 'sum.total', 'bool.total', 'sum.5', 'foo.bool.5','𐐀']) {
        it(`preserves ${name} as a metric name`, () => {
            expect(strictParser.parse(name).toString()).toBe('PromQL(VectorSelector(Identifier))');
        });
    }
    // Before dots were name characters, a dot ended a keyword, so a keyword that can
    // precede a number could be written directly before one such as .5.
    for (const [query, sameAs] of [
        // As a name, bool.5 would silently compare with a series instead of 0.5.
        ['foo > bool.5', 'foo > bool .5'],
        // Offset is a keyword but not an operator.
        ['foo offset.5', 'foo offset .5'],
        // Binary operators that are keywords take numbers too.
        ['foo atan2.5', 'foo atan2 .5'],
        // Keywords are case insensitive.
        ['foo > BOOL.5', 'foo > bool .5'],
    ]) {
        it(`parses ${query} like ${sameAs}`, () => {
            expect(strictParser.parse(query).toString()).toBe(strictParser.parse(sameAs).toString());
        });
    }
});
