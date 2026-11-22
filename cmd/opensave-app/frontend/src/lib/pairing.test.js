import { describe, it, expect } from 'vitest';
import { waitingOn, checkDigits, ASK_WINDOW_MS, addressProblem } from './pairing.js';

describe('checkDigits', () => {
  it('splits four digits', () => {
    expect(checkDigits('0482')).toEqual(['0', '4', '8', '2']);
  });
  it('gives nothing for a missing or malformed code', () => {
    for (const c of [undefined, null, '', '482', '48210', 'abcd', 4821]) {
      expect(checkDigits(c)).toEqual(c === 4821 ? ['4', '8', '2', '1'] : []);
    }
  });
});

describe('waitingOn', () => {
  const at = 1_000_000;
  const map = { deck: { name: 'Steam Deck', code: '4821', at } };
  it('is the request while the other device can still accept', () => {
    expect(waitingOn(map, 'deck', at + ASK_WINDOW_MS - 1)?.code).toBe('4821');
  });
  it('is nothing once the window has gone, or for a device never asked', () => {
    expect(waitingOn(map, 'deck', at + ASK_WINDOW_MS)).toBeNull();
    expect(waitingOn(map, 'laptop', at)).toBeFalsy();
    expect(waitingOn(undefined, 'deck', at)).toBeFalsy();
  });
});

describe('addressProblem', () => {
  it('takes IP addresses and names', () => {
    for (const ip of ['192.168.1.42', 'steamdeck.local', 'my-pc', 'fe80::1', '[fe80::1]', '  10.0.0.5  ']) {
      expect(addressProblem(ip, 8383), ip).toBe('');
    }
  });
  it('says nothing until something is typed', () => {
    expect(addressProblem('', 8383)).toBe('');
    expect(addressProblem('   ', 8383)).toBe('');
  });
  it('refuses what is no address', () => {
    for (const ip of ['not an address', '999.1.1.1', 'pc_name', 'http://192.168.1.2', '192.168.1.2:8383']) {
      expect(addressProblem(ip, 8383), ip).toBe('address');
    }
  });
  it('refuses a port out of range', () => {
    for (const port of [0, 70000, '', 'x', 1.5]) expect(addressProblem('192.168.1.2', port), String(port)).toBe('port');
  });
});
