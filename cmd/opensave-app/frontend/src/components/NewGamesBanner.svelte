<script>
  // Games installed since the last look, with saves nothing is keeping yet.
  //
  // The save scan only ran when someone pressed the button, so a new game sat
  // untracked until they thought to — usually after losing something. The
  // daemon now scans in the background and says so here. It never tracks on
  // its own: Review opens the scan, where the person chooses.
  import { fly as flyIn } from 'svelte/transition';
  import { gated } from '../lib/motion.js';
  // Instant with animations off; see lib/motion.js.
  const fly = gated(flyIn);
  import { newGames, navigate, toast } from '../lib/stores.js';
  import { notifyPrefs } from '../lib/notifyprefs.js';
  import { api } from '../lib/api.js';
  import { t, segments } from '../lib/i18n.js';
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
    const all = rest > 0 ? [...items, $t('newGames.more', { n: rest })] : items;
    if (all.length === 1) return all[0];
    return all.slice(0, -1).join($t('newGames.listSep')) + $t('newGames.listAnd') + all[all.length - 1];
  };
</script>

{#if $newGames.length > 0 && $notifyPrefs.newGames}
  <div class="new-card" transition:fly={{ y: -20, duration: 200 }}>
    <div class="new-icon"><Gamepad2 size={19} /></div>
    <div class="new-body">
      <div class="new-title">
        {#if names.length === 1}
          {#each segments($t('newGames.titleOne'), { name: names[0] }, ['name']) as part}{#if part.bold}<strong>{part.text}</strong>{:else}{part.text}{/if}{/each}
        {:else}
          {#each segments($t('newGames.titleMany'), { count: names.length }, ['count']) as part}{#if part.bold}<strong>{part.text}</strong>{:else}{part.text}{/if}{/each}
        {/if}
      </div>
      <div class="new-sub">
        {#if names.length > 1}{list(shown, more)} · {/if}{$t('newGames.subNothing')}
      </div>
    </div>
    <div class="new-actions">
      <button class="btn small" disabled={busy} on:click={dismiss}>{$t('common.dismiss')}</button>
      <button class="btn small primary" disabled={busy} on:click={review}>{$t('newGames.review')}</button>
    </div>
  </div>
{/if}

<style>
  .new-card {
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 10px 14px;
    background: var(--bg-raised);
    border: 1px solid var(--accent);
    border-radius: var(--radius-lg);
    box-shadow: 0 12px 40px rgba(0, 0, 0, 0.55), 0 0 0 1px var(--accent-soft);
  }
  .new-icon {
    flex-shrink: 0;
    width: 36px;
    height: 36px;
    border-radius: calc(10px * var(--round, 1));
    display: grid;
    place-items: center;
    background: var(--accent-soft);
    color: var(--accent-text);
  }
  /* The buttons go under the text when a narrow window leaves the text
     too little room beside them. */
  .new-card {
    flex-wrap: wrap;
    row-gap: 8px;
  }
  .new-body {
    flex: 1 1 320px;
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
  /* Side by side: stacked, the two buttons made the card twice the height
     of its text. */
  .new-actions {
    display: flex;
    gap: 8px;
    margin-left: auto;
    flex-shrink: 0;
  }
</style>
