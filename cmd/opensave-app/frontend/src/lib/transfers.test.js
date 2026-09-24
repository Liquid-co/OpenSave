import { describe, expect, it } from 'vitest';
import { amountLabel, peerLabel, speedLabel, withNames } from './transfers.js';

describe('transfer wording', () => {
  it('shows speed only when there is one', () => {
    expect(speedLabel(1048576 * 1.5)).toBe('1.5 MB/s');
    expect(speedLabel(0)).toBe('');
    expect(speedLabel(undefined)).toBe('');
  });

  it('shows how far along a running transfer is, and the size of a finished one', () => {
    expect(amountLabel({ state: 'running', bytesTransferred: 409600, totalBytes: 1048576 })).toBe('400.0 KB of 1.0 MB');
    expect(amountLabel({ state: 'done', bytesTransferred: 1048576, totalBytes: 1048576 })).toBe('1.0 MB');
    expect(amountLabel({ state: 'error', bytesTransferred: 2048, totalBytes: 0 })).toBe('2.0 KB');
    expect(amountLabel({ state: 'done', bytesTransferred: 0, totalBytes: 0 })).toBe('');
  });

  it('says which way it went', () => {
    expect(peerLabel({ direction: 'download', peer: 'Deck' })).toBe('from Deck');
    expect(peerLabel({ direction: 'upload', peer: 'Deck' })).toBe('to Deck');
    expect(peerLabel({ direction: 'download' })).toBe('from another device');
  });

  it("names each game, falling back to its id for one this device doesn't track", () => {
    const named = withNames(
      { active: [{ gameId: 'hades' }], recent: [{ gameId: 'gone' }] },
      { hades: { name: 'Hades' } }
    );
    expect(named.active[0].name).toBe('Hades');
    expect(named.recent[0].name).toBe('gone');
    expect(withNames(null, {})).toEqual({ active: [], recent: [] });
  });
});
