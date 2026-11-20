<script>
  // The strip at the top of the Library when something is waiting on you:
  // how many of each kind, and Review, which opens the bell's list (the same
  // list, counted — lib/libraryhome.js waitingSummary). Kinds that have a
  // banner of their own above the page (new games found, a newer save from
  // the cloud) or a card of their own (a device asking to pair) are left to
  // it rather than said twice. And the snapshots' drive nearly full, which
  // nothing else on screen would say.
  import { onDestroy } from 'svelte';
  import TriangleAlert from 'lucide-svelte/icons/triangle-alert';
  import HardDrive from 'lucide-svelte/icons/hard-drive';
  import { games, conflicts, locationConflicts, pairingRequests, cloudOffers, newGames, navigate } from '../../lib/stores.js';
  import { waitingOnYou, openBell } from '../../lib/notifications.js';
  import { waitingSummary } from '../../lib/libraryhome.js';
  import { api } from '../../lib/api.js';
  import { fmtSize } from '../../lib/format.js';
  import { t } from '../../lib/i18n.js';
  import { collapse } from '../../lib/motion.js';

  const ELSEWHERE = new Set(['new', 'offer', 'pair']);

  $: kinds = waitingSummary(
    waitingOnYou({ games: $games, conflicts: $conflicts, locationConflicts: $locationConflicts, pairingRequests: $pairingRequests, cloudOffers: $cloudOffers, newGames: $newGames })
  ).filter((k) => !ELSEWHERE.has(k.kind));
  $: total = kinds.reduce((sum, k) => sum + k.n, 0);

  // The drive the snapshots are kept on, when it is nearly full: a snapshot
  // that cannot be written is a save that is not backed up. Asked for now and
  // every few minutes.
  const LOW_SPACE = 1024 ** 3;
  let lowSpace = null;
  async function checkSpace() {
    try {
      const r = await api.get('/api/storage');
      lowSpace = r.freeKnown && r.freeBytes < LOW_SPACE ? r.freeBytes : null;
    } catch {
      lowSpace = null;
    }
  }
  checkSpace();
  const spaceTick = setInterval(checkSpace, 5 * 60_000);
  onDestroy(() => clearInterval(spaceTick));
</script>

{#if total > 0}
  <section class="strip" aria-label={$t('home.needs.label')} transition:collapse>
    <TriangleAlert size={16} class="mark" />
    <span class="head">{$t('home.needs.title', { n: total })}</span>
    <span class="kinds">
      {#each kinds as k (k.kind)}
        <span class="kind"><b>{#key k.n}<span class="num">{k.n}</span>{/key}</b>{$t('home.needs.kind.' + k.kind, { n: k.n })}</span>
      {/each}
    </span>
    <span class="grow"></span>
    <button class="btn small" on:click={() => openBell.set(true)}>{$t('home.needs.review')}</button>
  </section>
{/if}
{#if lowSpace !== null}
  <section class="strip" aria-label={$t('home.needs.spaceLabel')} transition:collapse>
    <HardDrive size={16} class="mark" />
    <span class="head">{$t('home.summary.lowSpace', { size: fmtSize(lowSpace) })}</span>
    <span class="grow"></span>
    <button class="btn small" on:click={() => navigate('settings', { tab: 'storage' })}>{$t('home.summary.seeSpace')}</button>
  </section>
{/if}

<style>
  .strip {
    display: flex;
    align-items: center;
    gap: 12px;
    min-height: 48px;
    padding: 6px 8px 6px 16px;
    border-radius: var(--radius-lg);
    background: rgba(var(--warn-rgb), 0.08);
    border: 1px solid rgba(var(--warn-rgb), 0.3);
    flex-wrap: wrap;
  }
  .strip :global(.mark) {
    color: var(--warn);
    flex-shrink: 0;
  }
  .head {
    font-weight: 600;
  }
  .kinds {
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
  }
  .kind {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    height: 26px;
    padding: 0 10px;
    border-radius: 999px;
    background: var(--bg-raised);
    border: 1px solid var(--border-strong);
    font-size: var(--text-md);
  }
  .kind b {
    font-weight: 700;
  }
  /* A count that changes ticks over rather than just being different. */
  .num {
    display: inline-block;
    animation: num-in 0.24s cubic-bezier(0.2, 0.7, 0.2, 1.4) backwards;
  }
  @keyframes num-in {
    from {
      opacity: 0;
      transform: translateY(-5px);
    }
  }
  .grow {
    flex: 1;
  }
</style>
