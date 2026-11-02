import { tr } from './i18n.js';
// Pausing syncing, from the app: the wording, and the calls. The state is the
// daemon's (see internal/syncpause); the app shows it and asks for changes.
import { api } from './api.js';
import { toast } from './stores.js';

/** The lengths offered wherever pausing is, worded to follow "Pause
 *  syncing". null is until resumed. */
export const PAUSE_CHOICES = [
  { minutes: 15, get label() { return tr('syncpause.for15Minutes'); } },
  { minutes: 60, get label() { return tr('syncpause.for1Hour'); } },
  { minutes: 180, get label() { return tr('syncpause.for3Hours'); } },
  { minutes: null, get label() { return tr('syncpause.untilIResume'); } }
];

/** "for 42 more minutes", "for 1 h 05 min", "until you resume". */
export function pauseLength(pause, now = Date.now()) {
  if (!pause?.paused) return '';
  if (pause.untilRestart || !pause.endsAt) return tr('syncpause.untilYouResume');
  const minutes = Math.max(0, Math.ceil((pause.endsAt - now) / 60000));
  if (minutes >= 60) {
    const h = Math.floor(minutes / 60);
    const m = minutes % 60;
    const hours = h === 1 ? tr('syncpause.1Hour') : tr('syncpause.hours', { p0: h });
    return m ? tr('syncpause.for', { p0: hours, p1: m === 1 ? tr('time.oneMinute') : tr('time.minutes', { n: m }) }) : tr('syncpause.forLabel', { p0: hours });
  }
  if (minutes <= 1) return tr('syncpause.forLessThanAMinute');
  return tr('syncpause.forMoreMinutes', { p0: minutes });
}

/** Short form for the status bar: "42 min left", "until resumed". */
export function pauseShort(pause, now = Date.now()) {
  if (!pause?.paused) return '';
  if (pause.untilRestart || !pause.endsAt) return tr('syncpause.untilResumed');
  const minutes = Math.max(0, Math.ceil((pause.endsAt - now) / 60000));
  if (minutes >= 60) {
    const h = Math.floor(minutes / 60);
    const m = minutes % 60;
    return m ? tr('syncpause.hMinLeft', { p0: h, p1: m }) : tr('syncpause.hLeft', { p0: h });
  }
  return tr('syncpause.minLeft', { p0: Math.max(1, minutes) });
}

export async function pauseSync(minutes) {
  try {
    await api.post('/api/sync/pause', minutes ? { minutes } : { untilRestart: true });
    toast(
      minutes ? tr('syncpause.syncingPausedSnapshotsAreStillTaken') : tr('syncpause.syncingPausedUntilYouResumeSnapshots'),
      'success'
    );
  } catch (e) {
    toast(e.message, 'error');
  }
}

export async function resumeSync() {
  try {
    await api.post('/api/sync/resume', {});
    toast(tr('syncpause.syncingResumedCatchingUp'), 'success');
  } catch (e) {
    toast(e.message, 'error');
  }
}
