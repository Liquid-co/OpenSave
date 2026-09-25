// Settings that save themselves as they are changed, instead of on a button
// that was easy to forget and meant every toggle was two clicks.
//
// The page keeps an editable copy (the draft) and the last values the daemon
// confirmed (saved). What is sent is only what differs between the two: the
// daemon applies a partial update over what it has, so a field this page
// never touched is never sent — and a change made elsewhere since the page
// opened (the tray, the CLI, the Cloud Backup page) is not put back.

const same = (a, b) => JSON.stringify(a) === JSON.stringify(b);

/**
 * The fields of `draft` that differ from `saved`, as a patch; null when there
 * are none. `rejected` holds values the daemon refused, left out until the
 * field changes again, so one bad field does not hold every other change
 * hostage.
 */
export function changedFields(saved, draft, { rejected = {} } = {}) {
  if (!saved || !draft) return null;
  const patch = {};
  for (const [key, value] of Object.entries(draft)) {
    if (same(value, saved[key])) continue;
    if (key in rejected && same(value, rejected[key])) continue;
    patch[key] = value;
  }
  return Object.keys(patch).length ? patch : null;
}

/**
 * Brings values changed elsewhere into a draft, for the fields the person has
 * not touched here (draft still equal to saved). Returns the new draft and
 * saved copies.
 */
export function adoptOutsideChanges(saved, draft, incoming) {
  if (!saved || !draft || !incoming) return { saved, draft };
  const moved = Object.entries(incoming).filter(([key, value]) => !same(value, saved[key]));
  // The same objects when nothing moved, so the page does not redraw for it.
  if (moved.length === 0) return { saved, draft };
  const nextSaved = { ...saved };
  const nextDraft = { ...draft };
  for (const [key, value] of moved) {
    if (same(draft[key], saved[key])) nextDraft[key] = structuredClone(value);
    nextSaved[key] = structuredClone(value);
  }
  return { saved: nextSaved, draft: nextDraft };
}

/**
 * One save at a time, in order; each collects what has changed when its turn
 * comes, so changes made while one is in flight go in the next.
 *
 * collect() → patch or null; send(patch) → the daemon's settings;
 * onSaved(result, patch) and onFailed(patch, error) report back; onState
 * gets 'saving' | 'saved' | {error}.
 */
export function createAutosave({ collect, send, onSaved, onFailed, onState = () => {} }) {
  let chain = Promise.resolve();
  let timer = null;

  async function saveOnce(patch) {
    try {
      onSaved(await send(patch), patch);
      return null;
    } catch (e) {
      return e;
    }
  }

  async function run() {
    const patch = collect();
    if (!patch) return;
    onState('saving');
    let error = await saveOnce(patch);
    // Refused as a whole: find which field it was, so the rest still save.
    if (error && Object.keys(patch).length > 1) {
      error = null;
      for (const [key, value] of Object.entries(patch)) {
        const e = await saveOnce({ [key]: value });
        if (e) {
          onFailed({ [key]: value }, e);
          error = e;
        }
      }
    } else if (error) {
      onFailed(patch, error);
    }
    onState(error ? { error: error.message } : 'saved');
  }

  /** Saves now; resolves when this and every earlier save are done. */
  function flush() {
    clearTimeout(timer);
    timer = null;
    chain = chain.then(run);
    return chain;
  }

  /** Saves after `ms`, restarting the wait on each call. */
  function soon(ms) {
    clearTimeout(timer);
    timer = setTimeout(flush, ms);
  }

  return { flush, soon };
}
