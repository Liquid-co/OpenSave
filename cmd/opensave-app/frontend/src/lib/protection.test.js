import { describe, it, expect } from 'vitest';
import { protectionState, roomProtection, PROTECTION_LABELS } from './protection.js';

// The rule these tests exist to hold: the badge may never claim more
// protection than the sender actually applies. Everything else is wording.

describe('protectionState', () => {
  it('reports a relay pairing with a proven key as encrypted', () => {
    expect(protectionState({ overRelay: true, hasKey: true, encrypted: true })).toBe('sealed');
  });

  it('does not claim encryption from a key alone', () => {
    // A key exists but the peer has not proved it holds the matching half, so
    // the sender is not sealing yet. Saying "encrypted" here would describe
    // traffic that is going out in the clear.
    expect(protectionState({ overRelay: true, hasKey: true, encrypted: false })).toBe('pending');
  });

  it('reports a relay pairing with no key as not encrypted', () => {
    expect(protectionState({ overRelay: true, hasKey: false, encrypted: false })).toBe('open');
  });

  it('reports a local device as a direct connection rather than a failure', () => {
    // Nothing passes through a relay on this route, so "not encrypted" would
    // be true and would give exactly the wrong impression.
    expect(protectionState({ overRelay: false, hasKey: true, encrypted: false })).toBe('direct');
  });

  it('falls back to the address when an older daemon sends no protection fields', () => {
    expect(protectionState({ address: '192.168.1.5' })).toBe('direct');
    // Unknown, so the cautious answer — never "encrypted" on no evidence.
    expect(protectionState({ address: 'relay' })).toBe('open');
  });

  it('never reports encryption for a peer it knows nothing about', () => {
    for (const peer of [undefined, null, {}]) {
      expect(protectionState(peer)).not.toBe('sealed');
    }
  });
});

describe('roomProtection', () => {
  const sealed = { name: 'Deck', overRelay: true, hasKey: true, encrypted: true };
  const open = { name: 'Old PC', overRelay: true, hasKey: false, encrypted: false };
  const pending = { name: 'Laptop', overRelay: true, hasKey: true, encrypted: false };
  const lan = { name: 'Desktop', overRelay: false };

  it('counts only devices reached through a relay', () => {
    const { relayPeers } = roomProtection([sealed, open, lan]);
    expect(relayPeers).toHaveLength(2);
  });

  it('separates the encrypted from the ones worth acting on', () => {
    const { encrypted, unprotected } = roomProtection([sealed, open]);
    expect(encrypted.map((p) => p.name)).toEqual(['Deck']);
    expect(unprotected.map((p) => p.name)).toEqual(['Old PC']);
  });

  it('does not ask the user to re-pair a device that is about to encrypt itself', () => {
    // 'pending' clears in seconds on its own. Listing it as unprotected would
    // send someone through an unpair-and-pair cycle for nothing.
    const { unprotected } = roomProtection([pending]);
    expect(unprotected).toHaveLength(0);
  });

  it('says nothing at all when no device is reached through a relay', () => {
    const { relayPeers } = roomProtection([lan]);
    expect(relayPeers).toHaveLength(0);
  });

  it('handles an empty or missing peer list', () => {
    expect(roomProtection([]).relayPeers).toHaveLength(0);
    expect(roomProtection(undefined).relayPeers).toHaveLength(0);
  });
});

describe('wording', () => {
  it('has a label for every state', () => {
    for (const state of ['sealed', 'pending', 'open', 'direct']) {
      expect(PROTECTION_LABELS[state]).toBeTruthy();
    }
  });

  it('does not use the word encrypted for a state that is not', () => {
    expect(PROTECTION_LABELS.open.toLowerCase()).toContain('not encrypted');
    expect(PROTECTION_LABELS.direct.toLowerCase()).not.toContain('encrypted');
  });
});
