<script>
  // Games installed since the last look, with saves nothing is keeping yet.
  //
  // The save scan only ran when someone pressed the button, so a new game sat
  // untracked until they thought to — usually after losing something. The
  // daemon now scans in the background and says so here. It never tracks on
  // its own: Review opens the scan, where the person chooses.
  import { fly } from 'svelte/transition';
  import { newGames, navigate, toast } from '../lib/stores.js';
  import { api } from '../lib/api.js';
  import Gamepad2 from 'lucide-svelte/icons/gamepad-2';

  let busy = false;

  $: names = $newGames.map((g) => g.name);
  $: shown = names.slice(0, 3);
  $: more = names.length - shown.length;

  async function dismiss() {
    if (busy) return;
    busy = true;
    try {
      await api.post('/api/presets/new/dismiss', {});
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      busy = false;
    }
  }

  async function review() {
    // The scan lists them with everything else, so the card has done its job.
    navigate('home', { scan: Date.now() });
    await dismiss();
  }

  const list = (items, rest) => {
    const all = rest > 0 ? [...items, `${rest} more`] : items;
    if (all.length === 1) return all[0];
    return all.slice(0, -1).join(', ') + ' and ' + all[all.length - 1];
  };
</script>

{#if $newGames.length > 0}
  <div class="new-card" transition:fly={{ y: -20, duration: 200 }}>
    <div class="new-icon"><Gamepad2 size={19} /></div>
    <div class="new-body">
      <div class="new-title">
        {#if names.length === 1}
          <strong>{names[0]}</strong> has saves OpenSave isn't keeping yet
        {:else}
          <strong>{names.length} games</strong> have saves OpenSave isn't keeping yet
        {/if}
      </div>
      <div class="new-sub">
        {#if names.length > 1}{list(shown, more)} · {/if}nothing is tracked until you choose it
      </div>
    </div>
    <div class="new-actions">
      <button class="btn small" disabled={busy} on:click={dismiss}>Dismiss</button>
      <button class="btn small primary" disabled={busy} on:click={review}>Review</button>
    </div>
  </div>
{/if}

<style>
  .new-card {
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 13px 16px;
    background: var(--bg-raised);
    border: 1px solid var(--accent);
    border-radius: var(--radius-lg);
    box-shadow: 0 12px 40px rgba(0, 0, 0, 0.55), 0 0 0 1px var(--accent-soft);
  }
  .new-icon {
    flex-shrink: 0;
    width: 36px;
    height: 36px;
    border-radius: 10px;
    display: grid;
    place-items: center;
    background: var(--accent-soft);
    color: var(--accent);
  }
  .new-body {
    flex: 1;
    min-width: 0;
  }
  .new-title {
    font-size: 0.92rem;
  }
  .new-sub {
    font-size: 0.78rem;
    color: var(--text-faint);
    margin-top: 2px;
  }
  .new-actions {
    display: flex;
    flex-direction: column;
    gap: 6px;
    flex-shrink: 0;
  }
</style>
