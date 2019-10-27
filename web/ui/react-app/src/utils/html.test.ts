import { escapeHTML } from './html';

describe('escapeHTML', (): void => {
  it('escapes html sequences', () => {
    expect(escapeHTML(`<strong>'example'&"another/example"</strong>`)).toEqual(
      '&lt;strong&gt;&#39;example&#39;&amp;&quot;another&#x2F;example&quot;&lt;&#x2F;strong&gt;'
    );
  });
});
