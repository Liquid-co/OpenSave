<script>
  // Pausing syncing, where it can be seen: Home's own row of actions. The
  // same pause as the status bar's (lib/syncpause.js); while it lasts this
  // turns into the way back, with the time left on it.
  import { onDestroy } from 'svelte';
  import Pause from 'lucide-svelte/icons/pause';
  import Play from 'lucide-svelte/icons/play';
  import { syncPause } from '../lib/stores.js';
  import { openMenu } from '../lib/contextmenu.js';
  import { PAUSE_CHOICES, pauseSync, resumeSync, pauseShort, pauseLength } from '../lib/syncpause.js';

  let now = Date.now();
  const clock = setInterval(() => (now = Date.now()), 15_000);
  onDestroy(() => clearInterval(clock));
  $: if ($syncPause) now = Date.now();

  const choose = (e) =>
    openMenu(
      e,
      PAUSE_CHOICES.map((c) => ({ label: `Pause ${c.label}`, icon: Pause, run: () => pauseSync(c.minutes) }))
    );
</script>

{#if $syncPause.paused}
  <button
    class="btn resume"
    on:click={resumeSync}
    title="Syncing is paused {pauseLength($syncPause, now)}. Snapshots are still taken. Resume now to catch up."
  >
    <Play size={15} />Resume syncing<span class="left">{pauseShort($syncPause, now)}</span>
  </button>
{:else}
  <button class="btn" on:click={choose} title="Stop syncing with your other devices for a while — snapshots are still taken">
    <Pause size={15} />Pause syncing
  </button>
{/if}

<style>
  .resume {
    border-color: rgba(var(--warn-rgb), 0.5);
    background: rgba(var(--warn-rgb), 0.1);
  }
  .left {
    margin-left: 4px;
    font-size: 0.8rem;
    font-weight: 500;
    color: var(--text-dim);
  }
</style>
