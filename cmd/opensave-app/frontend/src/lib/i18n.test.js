import { describe, expect, it } from 'vitest';

import en from '../locales/en.json';
import zhHans from '../locales/zh-Hans.json';
import zhHant from '../locales/zh-Hant.json';
import { LOCALES, loadLocale, pluralRulesFor, resolveLocale, sanitizeLocale, scriptFor, segments, translate } from './i18n.js';

// The catalogs are flat, dot-keyed objects; a plural value is one value that
// must expose an "other" form. Comparing top-level keys is enough.
const keysOf = (catalog) => Object.keys(catalog);

/** Every catalog, as [locale id, catalog]. A new language is one line here
 *  plus one in LOCALES; the checks below then cover it for free. */
const CATALOGS = [['en', en], ['zh-Hans', zhHans], ['zh-Hant', zhHant]];

describe('locale catalogs', () => {
  it('declares every catalog id in LOCALES', () => {
    for (const [id] of CATALOGS) expect(LOCALES[id], id).toBeTruthy();
  });

  it('has no key outside en.json in any catalog', () => {
    const enKeys = new Set(keysOf(en));
    for (const [name, catalog] of CATALOGS) {
      for (const key of keysOf(catalog)) expect(enKeys.has(key), `${name}: extra key: ${key}`).toBe(true);
    }
  });

  it('has a value for every key in en.json, in every catalog', () => {
    for (const [name, catalog] of CATALOGS) {
      const keys = new Set(keysOf(catalog));
      for (const key of keysOf(en)) expect(keys.has(key), `${name}: missing key: ${key}`).toBe(true);
    }
  });

  it('keeps every value non-empty, and plurals with an "other" form', () => {
    for (const [name, catalog] of CATALOGS) {
      for (const [key, v] of Object.entries(catalog)) {
        if (v !== null && typeof v === 'object') {
          expect(typeof v.other, `${name}:${key} plural needs "other"`).toBe('string');
          expect(v.other.trim().length, `${name}:${key}`).toBeGreaterThan(0);
        } else {
          expect(typeof v, `${name}:${key}`).toBe('string');
          expect(v.trim().length, `${name}:${key}`).toBeGreaterThan(0);
        }
      }
    }
  });
});

describe('translate', () => {
  const rules = pluralRulesFor('en');

  it('returns the key itself when nothing has it', () => {
    expect(translate({}, 'no.such.key', undefined, rules)).toBe('no.such.key');
  });

  it('falls back to English when the locale lacks the key', () => {
    expect(translate({ other: 'x' }, 'home.title', undefined, rules)).toBe('Home');
  });

  it('fills {params}', () => {
    expect(translate({ k: '{game}: got {count} files' }, 'k', { game: 'Hades', count: 7 }, rules)).toBe('Hades: got 7 files');
  });

  it('selects plural categories and always keeps an "other"', () => {
    const catalog = { k: { one: '{n} snapshot', other: '{n} snapshots' } };
    expect(translate(catalog, 'k', { n: 1 }, rules)).toBe('1 snapshot');
    expect(translate(catalog, 'k', { n: 3 }, rules)).toBe('3 snapshots');
    expect(translate(catalog, 'k', undefined, rules)).toBe('0 snapshots');
  });
});

describe('segments', () => {
  it('splits a template into plain and bold parts, in order', () => {
    expect(segments('open {a} then {b}.', { a: '设备', b: '云备份' }, ['b'])).toEqual([
      { text: 'open ', bold: false },
      { text: '设备', bold: false },
      { text: ' then ', bold: false },
      { text: '云备份', bold: true },
      { text: '.', bold: false }
    ]);
  });
});

describe('locale choice', () => {
  it('rejects unknown locale ids, and region tags that are not languages', () => {
    expect(sanitizeLocale('fr')).toBe(null);
    expect(sanitizeLocale('zh-CN')).toBe(null);
    expect(sanitizeLocale('zh-TW')).toBe(null);
    expect(sanitizeLocale('zh-Hans')).toBe('zh-Hans');
    expect(sanitizeLocale('zh-Hant')).toBe('zh-Hant');
    expect(sanitizeLocale('system')).toBe('system');
  });

  it("reports 'system' when the user has never picked one", () => {
    expect(loadLocale({ getItem: () => null })).toBe('system');
    expect(loadLocale({ getItem: () => 'zh-Hant' })).toBe('zh-Hant');
  });

  it('resolves system against the OS language, keeps explicit picks', () => {
    expect(resolveLocale('system', 'zh-CN')).toBe('zh-Hans');
    expect(resolveLocale('system', 'zh-TW')).toBe('zh-Hant');
    expect(resolveLocale('system', 'en-US')).toBe('en');
    expect(resolveLocale('system', '')).toBe('en');
    expect(resolveLocale('zh-Hant', 'en-US')).toBe('zh-Hant');
  });

  it('reads the Chinese script out of an OS language tag', () => {
    expect(scriptFor('zh-CN')).toBe('zh-Hans');
    expect(scriptFor('zh-SG')).toBe('zh-Hans');
    expect(scriptFor('zh-Hans-CN')).toBe('zh-Hans');
    expect(scriptFor('zh')).toBe('zh-Hans');
    expect(scriptFor('zh-TW')).toBe('zh-Hant');
    expect(scriptFor('zh-HK')).toBe('zh-Hant');
    expect(scriptFor('zh-MO')).toBe('zh-Hant');
    expect(scriptFor('zh-Hant-HK')).toBe('zh-Hant');
    expect(scriptFor('en-US')).toBe(null);
    expect(scriptFor('')).toBe(null);
  });

  it('survives storage that refuses to read', () => {
    const storage = { getItem: () => { throw new Error('blocked'); } };
    expect(loadLocale(storage)).toBe('system');
  });
});
