import { describe, expect, it } from 'vitest';
import { cloudTiles, connectedProviderOf, isEmail, providerStatus } from './cloudproviders.js';

describe('connectedProviderOf', () => {
  it('is the OAuth provider the tokens were issued for', () => {
    expect(connectedProviderOf({ provider: 'dropbox', tokens: { userEmail: 'a@b.co' } })).toBe('dropbox');
  });
  it('is nobody when the selected provider is not an OAuth one, tokens or not', () => {
    expect(connectedProviderOf({ provider: 'local', tokens: { userEmail: 'a@b.co' } })).toBe(null);
    expect(connectedProviderOf({ provider: 'webdav', tokens: { userEmail: 'a@b.co' } })).toBe(null);
  });
  it('is nobody without a sign-in', () => {
    expect(connectedProviderOf({ provider: 'google_drive' })).toBe(null);
    expect(connectedProviderOf(null)).toBe(null);
  });
});

describe('providerStatus', () => {
  const cfg = { provider: 'google_drive', url: '', tokens: { userEmail: 'me@example.com' } };
  it('shows the account on the connected card', () => {
    expect(providerStatus('google_drive', 'google_drive', cfg)).toBe('me@example.com');
  });
  it('says Connected when the provider would not reveal an address', () => {
    expect(providerStatus('dropbox', 'dropbox', { provider: 'dropbox', tokens: { userEmail: 'dropbox-user' } })).toBe('Connected');
  });
  it('invites a sign-in on the other OAuth cards', () => {
    expect(providerStatus('onedrive', 'google_drive', cfg)).toBe('Click to sign in');
  });
  it('says Configured only for the selected non-OAuth provider with a destination', () => {
    const local = { provider: 'local', url: 'D:\Backups' };
    expect(providerStatus('local', null, local)).toBe('Configured');
    expect(providerStatus('webdav', null, local)).toBe('Not configured');
    expect(providerStatus('local', null, { provider: 'local', url: '' })).toBe('Not configured');
  });
  it('says nothing before the settings load', () => {
    expect(providerStatus('local', null, null)).toBe('');
  });
});

describe('isEmail', () => {
  it('accepts an address and refuses a placeholder', () => {
    expect(isEmail('me@example.com')).toBe(true);
    expect(isEmail('dropbox-user')).toBe(false);
    expect(isEmail(undefined)).toBe(false);
  });
});

describe('cloudTiles', () => {
  const local = [
    { id: 'a', name: 'Alpha', coverUrl: 'x' },
    { id: 'b', name: 'Beta', coverUrl: '' }
  ];
  it('lists cloud games first, then tracked games with nothing in the cloud', () => {
    const tiles = cloudTiles([{ gameId: 'b', gameName: 'Beta' }, { gameId: 'z', gameName: 'Zed' }], local);
    expect(tiles.map((t) => [t.id, t.tracked, !!t.cloud])).toEqual([
      ['b', true, true],
      ['z', false, true],
      ['a', true, false]
    ]);
  });
  it('lists every tracked game when nothing is in the cloud yet', () => {
    expect(cloudTiles(null, local).map((t) => t.id)).toEqual(['a', 'b']);
  });
  it('asks for art from the tracked game, or from the name alone for a game only in the cloud', () => {
    const cover = (g) => `art:${g.id ?? '-'}:${g.name}`;
    const tiles = cloudTiles([{ gameId: 'b', gameName: 'Beta' }, { gameId: 'z', gameName: 'Zed' }], local, cover);
    expect(tiles.map((t) => t.coverUrl)).toEqual(['art:b:Beta', 'art:-:Zed', 'art:a:Alpha']);
  });
});
