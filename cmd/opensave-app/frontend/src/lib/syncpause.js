// Pausing syncing, from the app: the wording, and the calls. The state is the
// daemon's (see internal/syncpause); the app shows it and asks for changes.
import { api } from './api.js';
import { toast } from './stores.js';

/** The lengths offered wherever pausing is, worded to follow "Pause
 *  syncing". null is until resumed. */
export const PAUSE_CHOICES = [
  { minutes: 15, label: 'for 15 minutes' },
  { minutes: 60, label: 'for 1 hour' },
  { minutes: 180, label: 'for 3 hours' },
  { minutes: null, label: 'until I resume' }
];

/** "for 42 more minutes", "for 1 h 05 min", "until you resume". */
export function pauseLength(pause, now = Date.now()) {
  if (!pause?.paused) return '';
  if (pause.untilRestart || !pause.endsAt) return 'until you resume';
  const minutes = Math.max(0, Math.ceil((pause.endsAt - now) / 60000));
  if (minutes >= 60) {
    const h = Math.floor(minutes / 60);
    const m = minutes % 60;
    return m ? `for ${h} h ${String(m).padStart(2, '0')} min` : `for ${h} h`;
  }
  if (minutes <= 1) return 'for less than a minute';
  return `for ${minutes} more minutes`;
}

/** Short form for the status bar: "42 min left", "until resumed". */
export function pauseShort(pause, now = Date.now()) {
  if (!pause?.paused) return '';
  if (pause.untilRestart || !pause.endsAt) return 'until resumed';
  const minutes = Math.max(0, Math.ceil((pause.endsAt - now) / 60000));
  if (minutes >= 60) {
    const h = Math.floor(minutes / 60);
    const m = minutes % 60;
    return m ? `${h} h ${m} min left` : `${h} h left`;
  }
  return `${Math.max(1, minutes)} min left`;
}

export async function pauseSync(minutes) {
  try {
    await api.post('/api/sync/pause', minutes ? { minutes } : { untilRestart: true });
    toast(
      minutes ? `Syncing paused — snapshots are still taken` : 'Syncing paused until you resume — snapshots are still taken',
      'success'
    );
  } catch (e) {
    toast(e.message, 'error');
  }
}

export async function resumeSync() {
  try {
    await api.post('/api/sync/resume', {});
    toast('Syncing resumed — catching up', 'success');
  } catch (e) {
    toast(e.message, 'error');
  }
}
