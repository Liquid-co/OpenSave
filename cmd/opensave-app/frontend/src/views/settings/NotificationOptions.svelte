<script>
  // Which events interrupt you on this device. See lib/notifyprefs.js.
  import { notifyPrefs, NOTIFY_EVENTS } from '../../lib/notifyprefs.js';
  import { exampleEvents, exampleEvent } from '../../lib/notifications.js';
  import { playChime } from '../../lib/notify.js';
  import { toast } from '../../lib/stores.js';
  import BellRing from 'lucide-svelte/icons/bell-ring';

  // How each kind shows, played through once: a message in the corner with a
  // new entry behind the bell, then — a moment later — the chime that comes
  // with something that needs you (which also brings OpenSave to the front
  // when it is behind other windows).
  let showing = false;
  function showMe() {
    showing = true;
    toast('Hades: got 3 files from Steam Deck — an example', 'info', { action: { label: 'Open', run: () => {} } });
    exampleEvents.update((list) => [exampleEvent(), ...list]);
    setTimeout(() => {
      playChime();
      toast('That chime is for something that needs you — a conflict, an emptied save, a device asking to pair. OpenSave also comes to the front for it.', 'warning', { ttl: 9000 });
      showing = false;
    }, 2200);
  }

  const toggle = (key) => notifyPrefs.update((p) => ({ ...p, [key]: !p[key] }));
  const attention = Object.entries(NOTIFY_EVENTS).filter(([, e]) => e.kind === 'attention');
  const messages = Object.entries(NOTIFY_EVENTS).filter(([, e]) => e.kind === 'message');
</script>

<div class="notif">
<div class="demo">
  <p class="hint">
    Everything also goes behind the bell at the top of the window, which counts what's new and what is waiting on you.
  </p>
  <button class="btn small" disabled={showing} on:click={showMe}><BellRing size={14} />Show me</button>
</div>
<p class="lead">Chime and bring OpenSave to the front when…</p>
{#each attention as [key, e]}
  <label class="check">
    <input type="checkbox" checked={$notifyPrefs[key]} on:change={() => toggle(key)} />
    {e.label}
  </label>
{/each}

<p class="lead">Show a message when…</p>
{#each messages as [key, e]}
  <label class="check">
    <input type="checkbox" checked={$notifyPrefs[key]} on:change={() => toggle(key)} />
    {e.label}
  </label>
{/each}

<label class="check quiet">
  <input type="checkbox" checked={$notifyPrefs.quietWhilePlaying} on:change={() => toggle('quietWhilePlaying')} />
  Stay quiet while a full-screen game is running
</label>
<p class="hint">
  No chime and no window in front of your game. Whatever it was waits on screen for when you come back.
</p>
</div>

<style>
  /* Not .hint: a hint straight after a switch is pulled up to it (app.css),
     and these head the group below them instead. */
  .lead {
    margin: 14px 0 2px;
    font-size: 0.8rem;
    color: var(--text-faint);
  }
  .lead:first-child {
    margin-top: 0;
  }
  .quiet {
    margin-top: 12px;
  }
  .demo {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    margin-bottom: 12px;
    padding-bottom: 12px;
    border-bottom: 1px solid var(--border);
  }
  .demo .hint {
    margin: 0;
  }
</style>
