import { describe, expect, it } from 'vitest';
import { adoptOutsideChanges, changedFields, createAutosave } from './autosave.js';

describe('changedFields', () => {
  it('sends only what differs, compared by value', () => {
    const saved = { deviceName: 'PC', startOnBoot: false, customScanPaths: ['D:\\Games'] };
    expect(changedFields(saved, { ...saved })).toBe(null);
    expect(changedFields(saved, { ...saved, startOnBoot: true })).toEqual({ startOnBoot: true });
    expect(changedFields(saved, { ...saved, customScanPaths: ['D:\\Games'] })).toBe(null);
    expect(changedFields(saved, { ...saved, customScanPaths: ['D:\\Games', 'E:\\'] })).toEqual({
      customScanPaths: ['D:\\Games', 'E:\\']
    });
  });

  it('leaves out a refused value until it is changed again', () => {
    const saved = { relayUrl: 'wss://a', port: 1 };
    const draft = { relayUrl: 'ws://public', port: 2 };
    const rejected = { relayUrl: 'ws://public' };
    expect(changedFields(saved, draft, { rejected })).toEqual({ port: 2 });
    expect(changedFields(saved, { ...draft, relayUrl: 'wss://b' }, { rejected })).toEqual({ relayUrl: 'wss://b', port: 2 });
  });
});

describe('adoptOutsideChanges', () => {
  it('takes a change made elsewhere unless the field is being edited here', () => {
    const saved = { startOnBoot: false, deviceName: 'PC' };
    const draft = { startOnBoot: false, deviceName: 'Gaming P' };
    const next = adoptOutsideChanges(saved, draft, { startOnBoot: true, deviceName: 'Laptop' });
    expect(next.draft).toEqual({ startOnBoot: true, deviceName: 'Gaming P' });
    expect(next.saved).toEqual({ startOnBoot: true, deviceName: 'Laptop' });
  });
});

describe('createAutosave', () => {
  const setup = (sendImpl) => {
    let saved = { a: 1, b: 1, relayUrl: 'wss://ok' };
    let draft = { ...saved };
    const rejected = {};
    const states = [];
    const sent = [];
    const saver = createAutosave({
      collect: () => changedFields(saved, draft, { rejected }),
      send: async (patch) => {
        sent.push(patch);
        return sendImpl(patch, saved);
      },
      onSaved: (result) => (saved = { ...result }),
      onFailed: (patch) => Object.assign(rejected, patch),
      onState: (s) => states.push(s)
    });
    return { saver, states, sent, set: (patch) => (draft = { ...draft, ...patch }), saved: () => saved };
  };
  const accept = async (patch, saved) => ({ ...saved, ...patch });

  it('saves nothing when nothing changed', async () => {
    const t = setup(accept);
    await t.saver.flush();
    expect(t.sent).toEqual([]);
    expect(t.states).toEqual([]);
  });

  it('saves in order, one at a time, each with what changed by its turn', async () => {
    let release;
    const gate = new Promise((r) => (release = r));
    const t = setup(async (patch, saved) => {
      if (patch.a === 2) await gate;
      return { ...saved, ...patch };
    });
    t.set({ a: 2 });
    const first = t.saver.flush();
    await null; // the first save has started, and collected its change
    t.set({ b: 2 });
    const second = t.saver.flush();
    release();
    await Promise.all([first, second]);
    expect(t.sent).toEqual([{ a: 2 }, { b: 2 }]);
    expect(t.saved()).toMatchObject({ a: 2, b: 2 });
    expect(t.states.at(-1)).toBe('saved');
  });

  it('changes made before a save starts go out together', async () => {
    const t = setup(accept);
    t.set({ a: 3 });
    const one = t.saver.flush();
    t.set({ b: 3 });
    const two = t.saver.flush();
    await Promise.all([one, two]);
    expect(t.sent).toEqual([{ a: 3, b: 3 }]);
  });

  it('when a batch is refused, saves the good fields and reports the bad one', async () => {
    const t = setup(async (patch, saved) => {
      if ('relayUrl' in patch && patch.relayUrl.startsWith('ws:')) throw new Error('relay URL must use wss://');
      return { ...saved, ...patch };
    });
    t.set({ a: 5, relayUrl: 'ws://public' });
    await t.saver.flush();
    expect(t.saved()).toMatchObject({ a: 5, relayUrl: 'wss://ok' });
    expect(t.states.at(-1)).toEqual({ error: 'relay URL must use wss://' });
    // The refused value is not sent again with the next change.
    t.set({ b: 9 });
    await t.saver.flush();
    expect(t.sent.at(-1)).toEqual({ b: 9 });
  });
});
