import { describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import { folderName, listenForDrops } from './filedrop.js';
import { view } from './stores.js';

describe('folderName', () => {
  it('takes the last folder on either separator, ignoring a trailing one', () => {
    expect(folderName('C:\\Users\\me\\Saved Games\\Hades')).toBe('Hades');
    expect(folderName('C:\\Games\\Celeste\\')).toBe('Celeste');
    expect(folderName('/home/deck/.local/share/Stardew Valley/')).toBe('Stardew Valley');
    expect(folderName('D:/Saves/Balatro')).toBe('Balatro');
  });
  it('gives no name for a drive root or nothing at all', () => {
    expect(folderName('D:\\')).toBe('');
    expect(folderName('')).toBe('');
    expect(folderName(undefined)).toBe('');
  });
});

describe('listenForDrops', () => {
  it('opens the Track card with the first dropped folder', () => {
    let drop;
    const runtime = { OnFileDrop: vi.fn((cb) => (drop = cb)), OnFileDropOff: vi.fn() };
    const stop = listenForDrops(runtime);
    drop(10, 20, ['C:\\Saves\\Hades', 'C:\\Saves\\Other']);
    expect(get(view)).toMatchObject({ name: 'home', params: { add: true, path: 'C:\\Saves\\Hades' } });
    stop();
    expect(runtime.OnFileDropOff).toHaveBeenCalled();
  });
  it('does nothing without the desktop runtime', () => {
    expect(() => listenForDrops(undefined)()).not.toThrow();
  });
});
