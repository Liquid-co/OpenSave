<script>
  // Which events interrupt you on this device. See lib/notifyprefs.js.
  import { notifyPrefs, NOTIFY_EVENTS } from '../../lib/notifyprefs.js';

  const toggle = (key) => notifyPrefs.update((p) => ({ ...p, [key]: !p[key] }));
  const attention = Object.entries(NOTIFY_EVENTS).filter(([, e]) => e.kind === 'attention');
  const messages = Object.entries(NOTIFY_EVENTS).filter(([, e]) => e.kind === 'message');
</script>

<div class="notif">
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
</style>
