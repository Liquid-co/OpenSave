import { describe, it, expect } from 'vitest';
import { savePathChange } from './savepath.js';

describe('savePathChange', () => {
  const current = 'C:\\Users\\a\\AppData\\Local\\FactoryGame\\Saved\\SaveGames\\76561198000000001';

  it('sends a genuinely new folder', () => {
    const next = 'C:\\Users\\a\\AppData\\Local\\FactoryGame\\Saved\\SaveGames\\99999999999999999';
    expect(savePathChange(current, next)).toBe(next);
  });

  it('sends nothing when the folder is unchanged', () => {
    expect(savePathChange(current, current)).toBeNull();
  });

  // The important one. An empty value is not "track nothing" — validation
  // rejects an empty save path, and sending it would fail the whole PATCH,
  // so a cleared box must not take the App ID and snapshot limits down with it.
  it('sends nothing when the box was cleared', () => {
    expect(savePathChange(current, '')).toBeNull();
    expect(savePathChange(current, '   ')).toBeNull();
    expect(savePathChange(current, null)).toBeNull();
    expect(savePathChange(current, undefined)).toBeNull();
  });

  it('trims, so stray whitespace is not treated as a move', () => {
    expect(savePathChange(current, `  ${current}  `)).toBeNull();
  });

  it('trims the value it does send', () => {
    expect(savePathChange(current, '  D:\\Saves\\Game  ')).toBe('D:\\Saves\\Game');
  });

  // A game whose stored path is somehow missing must still accept one being
  // set, rather than comparing against undefined and sending nothing.
  it('treats a missing current path as settable', () => {
    expect(savePathChange(undefined, 'D:\\Saves\\Game')).toBe('D:\\Saves\\Game');
    expect(savePathChange(null, 'D:\\Saves\\Game')).toBe('D:\\Saves\\Game');
    expect(savePathChange('', 'D:\\Saves\\Game')).toBe('D:\\Saves\\Game');
  });

  // Unix-shaped paths go through the same decision — the folder is opaque
  // here, and the daemon is what validates it.
  it('handles unix paths and unicode folders', () => {
    expect(savePathChange('/home/a/.local/share/g/111', '/home/a/.local/share/g/222')).toBe(
      '/home/a/.local/share/g/222'
    );
    expect(savePathChange('/Users/a/Library/g/1', '/Users/a/Library/g/1')).toBeNull();
    expect(savePathChange('/home/a/partidas', '/home/a/Épic Saves/compte un')).toBe(
      '/home/a/Épic Saves/compte un'
    );
  });

  // Case matters on Linux and macOS, so a case-only edit is a real move and
  // must be sent. The daemon decides whether it resolves to the same folder.
  it('treats a case-only change as a move', () => {
    expect(savePathChange('/home/a/Saves', '/home/a/saves')).toBe('/home/a/saves');
  });
});
