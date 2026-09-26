import { describe, expect, it } from 'vitest';
import { switchTitleId } from './switchtitle.js';
import { coverURL, gameCover } from './api.js';

describe('switchTitleId', () => {
  it('reads the title id of a Switch save folder, and nothing else', () => {
    expect(switchTitleId('C:\\Users\\a\\AppData\\Roaming\\citron\\nand\\user\\save\\0000000000000000\\8F3A\\0100f2c0115b6000')).toBe(
      '0100F2C0115B6000'
    );
    expect(switchTitleId('/home/deck/.local/share/eden/nand/user/save/0000000000000000/8f3a/0100F2C0115B6000/')).toBe('0100F2C0115B6000');
    expect(switchTitleId('/home/deck/.local/share/eden/nand/user/save/0100F2C0115B6000')).toBe('');
    expect(switchTitleId('/games/Hades/0100F2C0115B6000')).toBe('');
    expect(switchTitleId('')).toBe('');
    expect(switchTitleId(undefined)).toBe('');
  });
});

describe('a Switch game’s cover', () => {
  it('is asked for by its title id', () => {
    const game = {
      name: 'Tears of the Kingdom',
      savePath: '/x/nand/user/save/0000000000000000/8f3a/0100F2C0115B6000'
    };
    expect(gameCover(game)).toContain('titleId=0100F2C0115B6000');
    expect(gameCover(game, true)).toContain('portrait=1');
    expect(coverURL('', false, '', '0100F2C0115B6000')).toContain('titleId=0100F2C0115B6000');
    expect(gameCover({ name: 'Hades', appId: '1145360', savePath: '/x/Hades' })).not.toContain('titleId');
  });
});
