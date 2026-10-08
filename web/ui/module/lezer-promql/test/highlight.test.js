import { parser } from '../dist/index.es.js';
import { highlightTree, tagHighlighter, tags } from '@lezer/highlight';

const highlighter = tagHighlighter([
    { tag: tags.invalid, class: 'invalid' },
    { tag: tags.variableName, class: 'variable' },
    { tag: tags.labelName, class: 'label' },
    { tag: tags.string, class: 'string' },
]);

// Returns the highlighted ranges as `text:class`.
function highlight(expr) {
    const ranges = [];
    highlightTree(parser.parse(expr), highlighter, (from, to, classes) => ranges.push(`${expr.slice(from, to)}:${classes}`));
    return ranges;
}

describe('highlight', () => {
    const testCases = [
        { title: 'unquoted label name in a matcher', expr: 'foo{a="b"}', expected: ['foo:variable', 'a:label', '"b":string'] },
        { title: 'quoted label name in a matcher', expr: 'foo{"a.b"="c"}', expected: ['foo:variable', '"a.b":label', '"c":string'] },
        { title: 'quoted metric name', expr: '{"foo.bar"}', expected: ['"foo.bar":variable'] },
        { title: 'quoted metric name next to a quoted label name', expr: '{"foo.bar", "a.b"="c"}', expected: ['"foo.bar":variable', '"a.b":label', '"c":string'] },
        { title: 'quoted label name in a grouping', expr: 'sum by ("my.label", b) (x)', expected: ['"my.label":label', 'b:label', 'x:variable'] },
        { title: 'quoted label names in on and group_left', expr: 'a / on("x.y") group_left("z.w") b', expected: ['a:variable', '"x.y":label', '"z.w":label', 'b:variable'] },
        { title: 'string argument', expr: 'label_replace(x, "dst.x", "$1", "src", "(.*)")', expected: ['label_replace:variable', 'x:variable', '"dst.x":string', '"$1":string', '"src":string', '"(.*)":string'] },
        { title: 'unquoted name with a dot', expr: 'foo.bar{a="b"}', expected: ['foo:variable', '.:invalid', 'bar:invalid', 'a:label', '"b":string'] },
        { title: 'unquoted label name with a dot', expr: 'foo{a.b="c"}', expected: ['foo:variable', 'a:label', '.:invalid', 'b:invalid', '"c":string'] },
    ];
    testCases.forEach((value) => {
        it(value.title, () => {
            expect(highlight(value.expr).filter((r) => !/^[(){}[\],=]/.test(r))).toEqual(value.expected);
        });
    });
});
