// Every catalog key the app names in its code is in every catalog. A key
// missing shows on screen as the key itself ("settings.cloud.on"), which
// nothing else catches.
import { describe, it, expect } from 'vitest';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';
import en from '../locales/en.json';
import zhHans from '../locales/zh-Hans.json';
import zhHant from '../locales/zh-Hant.json';

const SRC = join(__dirname, '..');
function sources(dir, out = []) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) {
      if (name !== 'locales') sources(p, out);
    } else if ((name.endsWith('.svelte') || name.endsWith('.js')) && !name.endsWith('.test.js')) out.push(p);
  }
  return out;
}
// $t('x'), $i18n('x'), t('x'), tr('x'), get(t)('x'); 'x.' + y is a prefix.
const CALL = /(?:\$t|\$i18n|\bt|\btr|get\(t\))\(\s*'([a-zA-Z][a-zA-Z0-9_.]*)'(\s*\+)?/g;

describe('the catalogs', () => {
  const used = [];
  for (const file of sources(SRC)) {
    const text = readFileSync(file, 'utf8');
    for (const m of text.matchAll(CALL)) if (m[1].includes('.')) used.push({ key: m[1], prefix: !!m[2], file: relative(SRC, file) });
  }
  it('finds the keys it is checking', () => {
    expect(used.length).toBeGreaterThan(500);
  });
  for (const [name, cat] of Object.entries({ en, 'zh-Hans': zhHans, 'zh-Hant': zhHant })) {
    it(`has in ${name} every key the code names`, () => {
      const keys = Object.keys(cat);
      const missing = used.filter((u) => (u.prefix ? !keys.some((k) => k.startsWith(u.key)) : !(u.key in cat))).map((u) => `${u.key} (${u.file})`);
      expect([...new Set(missing)]).toEqual([]);
    });
  }
});
