import { describe, expect, it } from 'vitest';
import { sanitizeSidebar, pressSidebarButton, toggleSidebarHidden, chooseSidebar, sidebarShown, loadSidebar, saveSidebar, FOLD_BELOW } from './shell.js';

const memory = () => {
  const m = new Map();
  return { getItem: (k) => m.get(k) ?? null, setItem: (k, v) => m.set(k, v) };
};

describe('sidebar state', () => {
  it('starts full, and repairs anything stored wrong', () => {
    expect(sanitizeSidebar(undefined)).toEqual({ mode: 'full', lastShown: 'full' });
    expect(sanitizeSidebar({ mode: 'wide' })).toEqual({ mode: 'full', lastShown: 'full' });
    expect(sanitizeSidebar({ mode: 'hidden', lastShown: 'nonsense' })).toEqual({ mode: 'hidden', lastShown: 'full' });
    // A shown sidebar is the last one shown, whatever was stored beside it.
    expect(sanitizeSidebar({ mode: 'icons', lastShown: 'full' })).toEqual({ mode: 'icons', lastShown: 'icons' });
  });

  it('folds to icons and back with the title bar button', () => {
    const icons = pressSidebarButton({ mode: 'full' });
    expect(icons.mode).toBe('icons');
    expect(pressSidebarButton(icons).mode).toBe('full');
  });

  it('brings a hidden sidebar back as it was, from the button or Ctrl+B', () => {
    const hidden = toggleSidebarHidden({ mode: 'icons', lastShown: 'icons' });
    expect(hidden).toEqual({ mode: 'hidden', lastShown: 'icons' });
    expect(toggleSidebarHidden(hidden).mode).toBe('icons');
    expect(pressSidebarButton(hidden).mode).toBe('icons');
  });

  it('takes a mode picked by name, and ignores a made-up one', () => {
    expect(chooseSidebar({ mode: 'full' }, 'hidden')).toEqual({ mode: 'hidden', lastShown: 'full' });
    expect(chooseSidebar({ mode: 'hidden', lastShown: 'icons' }, 'hidden')).toEqual({ mode: 'hidden', lastShown: 'icons' });
    expect(chooseSidebar({ mode: 'icons' }, 'full')).toEqual({ mode: 'full', lastShown: 'full' });
    expect(chooseSidebar({ mode: 'icons' }, 'huge').mode).toBe('icons');
  });

  it('draws a full sidebar as icons in a narrow window, and leaves the others alone', () => {
    expect(sidebarShown({ mode: 'full' }, FOLD_BELOW - 1)).toBe('icons');
    expect(sidebarShown({ mode: 'full' }, FOLD_BELOW)).toBe('full');
    expect(sidebarShown({ mode: 'hidden' }, 800)).toBe('hidden');
    expect(sidebarShown({ mode: 'icons' }, 1920)).toBe('icons');
    expect(sidebarShown({ mode: 'full' }, undefined)).toBe('full');
  });

  it('is kept on this device, and survives storage that is broken', () => {
    const s = memory();
    saveSidebar({ mode: 'hidden', lastShown: 'icons' }, s);
    expect(loadSidebar(s)).toEqual({ mode: 'hidden', lastShown: 'icons' });
    expect(loadSidebar({ getItem: () => '{not json' })).toEqual({ mode: 'full', lastShown: 'full' });
    expect(() => saveSidebar({ mode: 'full' }, { setItem: () => { throw new Error('full'); } })).not.toThrow();
  });
});
