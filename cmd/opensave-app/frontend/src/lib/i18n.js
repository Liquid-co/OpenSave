// A tiny, dependency-free i18n layer. `t` is a Svelte store whose value is a
// translate function: in a component, `$t('key')` re-renders when the user
// switches language, with no framework wiring.
//
// Catalogs live in src/locales/ as flat, dot-keyed JSON. `en.json` is the
// source of truth: any key missing from another locale falls back to English,
// so a partial translation never shows a blank. A plural value is an object
// keyed by Intl.PluralRules categories (`{ "one": …, "other": … }`); Chinese
// and other single-form languages just use a string.
//
// The language choice is 'system' until the user picks one, and only an
// explicit pick is persisted — so an unpicked app keeps following the OS
// language as it changes.
import { writable, derived } from 'svelte/store';

import en from '../locales/en.json';
import zhHans from '../locales/zh-Hans.json';
import zhHant from '../locales/zh-Hant.json';

export const SYSTEM_LOCALE = 'system';

/** The languages offered besides the system default.
 *
 *  Chinese is split by script, not by region: which characters a reader wants
 *  is not the same question as where they are, so `zh-CN` / `zh-TW` / `zh-HK`
 *  and the rest are not offered as separate languages. Each region maps onto
 *  one of the two scripts here instead. */
export const LOCALES = { en: 'English', 'zh-Hans': '简体中文', 'zh-Hant': '繁體中文' };

const CATALOGS = { en, 'zh-Hans': zhHans, 'zh-Hant': zhHant };
const FALLBACK = 'en';
const KEY = 'opensave.locale';

/** A locale id if valid, otherwise null. */
export function sanitizeLocale(raw) {
  return typeof raw === 'string' && (raw === SYSTEM_LOCALE || raw in LOCALES) ? raw : null;
}

/** The saved choice, or 'system' when the user has never picked one. */
export function loadLocale(storage = globalThis.localStorage) {
  try {
    return sanitizeLocale(storage?.getItem(KEY)) ?? SYSTEM_LOCALE;
  } catch {
    return SYSTEM_LOCALE;
  }
}

/** Persist an explicit pick; 'system' clears it, following the OS again. */
export function saveLocale(l, storage = globalThis.localStorage) {
  try {
    if (l === SYSTEM_LOCALE) storage?.removeItem(KEY);
    else storage?.setItem(KEY, l);
  } catch {
    // Storage refused: the choice lasts until restart.
  }
}

/** Regions whose Chinese is written with traditional characters. Every other
 *  Chinese region — and a bare 'zh' — is read as simplified. */
const TRADITIONAL_REGIONS = ['tw', 'hk', 'mo'];

/** The Chinese script an OS/browser language tag is written in, or null when
 *  the tag is not Chinese. A tag that already names its script wins over the
 *  region guess. */
export function scriptFor(language) {
  const tag = (language ?? '').toLowerCase();
  if (!tag.startsWith('zh')) return null;
  if (tag.startsWith('zh-hant')) return 'zh-Hant';
  if (tag.startsWith('zh-hans')) return 'zh-Hans';
  return TRADITIONAL_REGIONS.includes(tag.split('-')[1]) ? 'zh-Hant' : 'zh-Hans';
}

/** 'system' settled against the OS language, or the explicit pick itself. */
export function resolveLocale(l, language = globalThis.navigator?.language) {
  if (l && l !== SYSTEM_LOCALE) return l;
  return scriptFor(language) ?? FALLBACK;
}

/** The chosen language. Setting it persists immediately ('system' unpersists). */
export const locale = writable(loadLocale());
locale.subscribe((l) => saveLocale(l));

/** Plural categories for a locale, as Intl.PluralRules sees them. */
export function pluralRulesFor(l) {
  try {
    return new Intl.PluralRules(l);
  } catch {
    return new Intl.PluralRules(FALLBACK);
  }
}

/** Look a key up with English fallback, resolve plurals, fill {params}. */
export function translate(catalog, key, params, rules) {
  let entry = catalog[key];
  if (entry === undefined) entry = CATALOGS[FALLBACK][key];
  if (entry === undefined) return key;
  if (entry !== null && typeof entry === 'object') {
    const category = rules.select(Number(params?.n ?? 0));
    entry = entry[category] ?? entry.other ?? key;
    // A plural template always mentions {n}; fill it even when the caller
    // passed no params at all.
    params = { n: 0, ...params };
  }
  if (params) {
    for (const [name, value] of Object.entries(params)) {
      entry = entry.replaceAll(`{${name}}`, String(value));
    }
  }
  return entry;
}

/** The translate function, rebound whenever the language changes. */
export const t = derived(locale, (l) => {
  const catalog = CATALOGS[resolveLocale(l)] ?? {};
  const rules = pluralRulesFor(resolveLocale(l));
  return (key, params) => translate(catalog, key, params, rules);
});

/** Split a translated template on its {placeholders}, marking which filled
 *  parts should render in bold. Returns [{ text, bold }, …] in order. */
export function segments(template, params = {}, boldNames = []) {
  const out = [];
  const re = /{(\w+)}/g;
  let last = 0;
  let m;
  while ((m = re.exec(template))) {
    if (m.index > last) out.push({ text: template.slice(last, m.index), bold: false });
    const name = m[1];
    out.push({ text: String(params[name] ?? m[0]), bold: boldNames.includes(name) });
    last = re.lastIndex;
  }
  if (last < template.length) out.push({ text: template.slice(last), bold: false });
  return out;
}
