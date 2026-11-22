import { describe, expect, it } from 'vitest';
import { noticeFor, updateAction } from './update.js';

const update = { available: true, latest: '2.5.1', current: '2.5.0', assetUrl: 'https://x/OpenSave.exe', url: 'https://x' };

describe('noticeFor', () => {
  it('says nothing when there is nothing to say', () => {
    expect(noticeFor({ page: 'home' })).toBeNull();
    expect(noticeFor({ update: { available: false }, page: 'home' })).toBeNull();
  });

  it('shows an update going in on every page, the What’s new page too', () => {
    const progress = { state: 'downloading', percentage: 42 };
    expect(noticeFor({ progress, update, page: 'changelog' })).toEqual({ kind: 'installing', state: 'downloading', percentage: 42 });
    expect(noticeFor({ progress: { state: 'error' }, update, page: 'home' }).kind).toBe('available');
  });

  it('offers a waiting update, but not on the What’s new page, which shows it, nor once put away', () => {
    expect(noticeFor({ update, page: 'home' })).toEqual({ kind: 'available', update });
    expect(noticeFor({ update, page: 'changelog' })).toBeNull();
    expect(noticeFor({ update, dismissed: true, page: 'home' })).toBeNull();
  });

  it('says what it was updated to, until the What’s new page is opened', () => {
    const updated = { to: '2.5.1', from: '2.5.0' };
    expect(noticeFor({ updated, page: 'home' })).toEqual({ kind: 'updated', to: '2.5.1', from: '2.5.0' });
    expect(noticeFor({ updated, page: 'changelog' })).toBeNull();
    // A newer one still waiting comes first.
    expect(noticeFor({ updated, update, page: 'home' }).kind).toBe('available');
  });
});

describe('updateAction', () => {
  it('installs in place when it can, else the Flatpak or a download', () => {
    expect(updateAction(update)).toBe('install');
    expect(updateAction({ ...update, assetUrl: '', flatpak: true })).toBe('flatpak');
    expect(updateAction({ ...update, assetUrl: '' })).toBe('download');
    expect(updateAction(null)).toBeNull();
  });
});
