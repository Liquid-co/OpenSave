// A newer OpenSave: whether there is one, what it changes, getting it, and
// the word after it is in. Shared by the notice at the top of the page
// (components/UpdateNotice.svelte) and the What's new page (views/Changelog),
// so the two always say the same thing and install the same way.
import { derived, get, writable } from 'svelte/store';
import { native } from './api.js';
import { appUpdate, availableUpdate, toast, view } from './stores.js';

/** Put away for this session; it comes back when the app next starts. */
export const updateDismissed = writable(false);

/** Set on the first start of a new build: {to, from}. */
export const justUpdated = writable(null);

/** True from Install being pressed until it fails or the app restarts. */
export const installStarted = writable(false);

/** How the update is got here: installed in place, the Flatpak, or a download. */
export function updateAction(update) {
  if (!update) return null;
  if (update.assetUrl) return 'install';
  if (update.flatpak) return 'flatpak';
  return 'download';
}

/** Does what the update button does: installs in place when it can, or
 *  opens the release page for the Flatpak or a download. */
export async function getUpdate() {
  const update = get(availableUpdate);
  const how = updateAction(update);
  if (!how) return;
  if (how !== 'install') {
    native.openExternal(update.url);
    return;
  }
  if (get(installStarted)) return;
  installStarted.set(true);
  const err = await native.installFromUrl(update.assetUrl);
  if (err) {
    toast(err, 'error');
    installStarted.set(false);
  }
}

/**
 * What the notice at the top of the page says, or null: an update being put
 * in (everywhere — it is happening), one waiting (not on the What's new page,
 * which shows it), or the word after one (not there either).
 * @returns {null | {kind: 'installing', state, percentage} | {kind: 'available', update} | {kind: 'updated', to, from}}
 */
export function noticeFor({ progress, update, dismissed, updated, page }) {
  if (progress && progress.state !== 'error') return { kind: 'installing', state: progress.state, percentage: progress.percentage ?? 0 };
  if (page === 'changelog') return null;
  if (update?.available && !dismissed) return { kind: 'available', update };
  if (updated?.to) return { kind: 'updated', to: updated.to, from: updated.from ?? '' };
  return null;
}

export const updateNotice = derived([appUpdate, availableUpdate, updateDismissed, justUpdated, view], ([progress, update, dismissed, updated, v]) =>
  noticeFor({ progress, update, dismissed, updated, page: v?.name })
);
