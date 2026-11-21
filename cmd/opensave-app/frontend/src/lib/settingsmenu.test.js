import { describe, it, expect } from 'vitest';
import { get } from 'svelte/store';
import { sectionFor, searchSettings, SETTINGS_INDEX, SECTIONS, settingsMenu, openSettings, closeSettings } from './settingsmenu.js';
import en from '../locales/en.json';

const t = (key) => en[key] ?? key;

describe('sectionFor', () => {
  it('takes a section, an old tab name, or starts at General', () => {
    expect(sectionFor({ section: 'appearance' })).toBe('appearance');
    expect(sectionFor({ tab: 'storage' })).toBe('storage');
    expect(sectionFor({ tab: 'support' })).toBe('about');
    expect(sectionFor({ tab: 'nonsense' })).toBe('general');
    expect(sectionFor()).toBe('general');
  });
});

describe('the menu', () => {
  it('opens on a section, and at a setting when asked, and closes', () => {
    openSettings({ tab: 'storage', focus: 'maxAuto' });
    expect(get(settingsMenu)).toEqual({ section: 'storage', focus: 'maxAuto' });
    closeSettings();
    expect(get(settingsMenu)).toBe(null);
  });
});

describe('SETTINGS_INDEX', () => {
  it('names a section that exists and words the catalog has, for every setting', () => {
    for (const e of SETTINGS_INDEX) {
      expect(SECTIONS).toContain(e.section);
      expect(en[e.label], e.label).toBeTruthy();
      if (e.hint) expect(en[e.hint], e.hint).toBeTruthy();
    }
    for (const s of SECTIONS) expect(en[`settings.section.${s}`], s).toBeTruthy();
  });
  it('gives every setting its own id', () => {
    const ids = SETTINGS_INDEX.map((e) => e.id);
    expect(new Set(ids).size).toBe(ids.length);
  });
});

describe('searchSettings', () => {
  it('finds a setting by the words of its name, any case', () => {
    const found = searchSettings('DEVICE name', t).map((e) => e.id);
    expect(found[0]).toBe('deviceName');
  });
  it('puts matches by name before matches in an explanation', () => {
    const found = searchSettings('snapshots', t);
    const firstHint = found.findIndex((e) => !t(e.label).toLowerCase().includes('snapshots'));
    const lastName = found.map((e) => t(e.label).toLowerCase().includes('snapshots')).lastIndexOf(true);
    expect(found.length).toBeGreaterThan(1);
    if (firstHint !== -1) expect(lastName).toBeLessThan(firstHint);
  });
  it('matches the start of a word, not the middle of one', () => {
    const found = searchSettings('port', t).map((e) => e.id);
    expect(found).toContain('port');
    expect(found).not.toContain('support');
  });
  it('matches writing without spaces anywhere in it', () => {
    const zh = { 'settings.advanced.daemonPort': '守护进程端口' };
    const tz = (k) => zh[k] ?? t(k);
    expect(searchSettings('端口', tz).map((e) => e.id)).toContain('port');
  });
  it('finds a setting by its section too, and nothing for nothing', () => {
    expect(searchSettings('controller', t).some((e) => e.section === 'controller')).toBe(true);
    expect(searchSettings('   ', t)).toEqual([]);
    expect(searchSettings('zzqqxx', t)).toEqual([]);
  });
});
