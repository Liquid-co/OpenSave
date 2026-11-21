<script>
  // Branches: separate saves for one game — a second playthrough, a run kept
  // apart, or another device's save kept when both changed it. Each says in
  // plain words where it came from (read from its first snapshot), its latest
  // save, and which devices are on it: this one, and the others as of their
  // last sync. One Switch button; deleting is in its ⋯ menu.
  //
  // Motion: the cards arrive one after another, and on a switch they move to
  // their new places (the one played first), the highlight passing to it.
  import { askConfirm, peers, settings } from '../../lib/stores.js';
  import { api } from '../../lib/api.js';
  import { snapshotKind } from '../../lib/snapshots.js';
  import { branchOrigin, latestOf, devicesOn } from '../../lib/gamepage.js';
  import { timeAgoParts } from '../../lib/timeago.js';
  import { gatedFlip, pop } from '../../lib/motion.js';
  import { t } from '../../lib/i18n.js';
  import NewBranchDialog from './NewBranchDialog.svelte';
  import Dropdown from '../../components/ui/Dropdown.svelte';
  import Plus from 'lucide-svelte/icons/plus';
  import GitBranch from 'lucide-svelte/icons/git-branch';
  import MoreHorizontal from 'lucide-svelte/icons/ellipsis';
  import Trash2 from 'lucide-svelte/icons/trash-2';

  export let game;
  export let runner;
  /** This device's name, as Activity knows it. */
  export let here = '';
  const { busy, run } = runner;

  $: hereName = here || $settings?.deviceName || '';
  $: branches = Object.values(game.branches ?? {})
    .map((b) => ({ b, origin: branchOrigin(b), latest: latestOf(b), on: devicesOn(b.name, game, $peers, hereName) }))
    .sort((x, y) => Number(y.b.name === game.activeBranch) - Number(x.b.name === game.activeBranch) || (y.latest?.timestamp ?? '').localeCompare(x.latest?.timestamp ?? ''));

  const day = (ms) => new Date(ms).toLocaleDateString(undefined, { day: 'numeric', month: 'short', ...(new Date(ms).getFullYear() === new Date().getFullYear() ? {} : { year: 'numeric' }) });
  const ago = (iso) => {
    const p = timeAgoParts(iso);
    if (p.kind === 'invalid') return p.text;
    const { kind, ...params } = p;
    return $t('timeAgo.' + kind, params);
  };
  // A branch kept from another device's save gets that device's name, not
  // the made-up name it was stored under.
  const title = (x) => (x.origin.kind === 'kept' && x.origin.peer ? $t('game.branches.keptName', { device: x.origin.peer, date: day(x.origin.ms) }) : x.b.name);
  function originText(o) {
    if (o.kind === 'main') return o.ms ? $t('game.branches.originMain', { date: day(o.ms) }) : $t('game.branches.originMainNoDate');
    if (o.kind === 'copied') return $t('game.branches.originCopied', { from: o.from, date: day(o.ms) });
    if (o.kind === 'kept') return $t('game.branches.originKept', { device: o.peer || $t('transfers.anotherDevice') });
    if (o.kind === 'made') return $t('game.branches.originMade', { date: day(o.ms) });
    return $t('game.branches.originEmpty');
  }
  const deviceNames = (on) => (on.length ? on.map((d) => (d.here ? $t('game.branches.thisPc') : d.name)).join(', ') : $t('game.branches.noDevices'));

  // ── Acting ──
  let dialogOpen = false;
  const createBranch = ({ name, copy }) => {
    dialogOpen = false;
    return run(copy ? $t('game.branches.createdFromCurrent') : $t('game.branches.createdEmpty'), () =>
      api.post(`/api/games/${game.id}/branch`, { name, copyCurrentSave: copy })
    );
  };
  const switchBranch = (name) => run($t('game.branches.switchedTo', { name }), () => api.post(`/api/games/${game.id}/branch/switch`, { name }));
  async function deleteBranch(name) {
    if (!(await askConfirm($t('game.branches.deleteConfirm', { name }), { title: $t('game.branches.deleteTitle'), confirmText: $t('common.delete'), danger: true }))) return;
    run($t('game.branches.deleted', { name }), () => api.del(`/api/games/${game.id}/branch/${encodeURIComponent(name)}`));
  }
  let menuFor = null;
</script>

<div class="intro">
  <p>{$t('game.branches.intro')}</p>
  <button class="btn" disabled={$busy} on:click={() => (dialogOpen = true)}><Plus size={15} />{$t('game.branches.newBranch')}</button>
</div>

<div class="list">
  {#each branches as x, i (x.b.name)}
    {@const active = x.b.name === game.activeBranch}
    <div class="branch" class:active style="--i: {Math.min(i, 8)}" animate:gatedFlip={{ duration: 360 }}>
      <span class="btile"><GitBranch size={18} /></span>
      <div class="main">
        <div class="name-row">
          <span class="name">{title(x)}</span>
          {#if x.origin.kind === 'kept'}<code class="real">{x.b.name}</code>{/if}
          {#if active}<span class="chip" in:pop={{ start: 0.7 }}>{$t('game.branches.playing')}</span>{/if}
        </div>
        <div class="origin">{originText(x.origin)}</div>
      </div>
      <div class="fact latest">
        <div class="cap">{$t('game.branches.latestSave')}</div>
        <div class="val">{x.latest ? $t('game.branches.latestValue', { title: snapshotKind(x.latest).title === 'Snapshot' ? $t('snapshots.kind.snapshot') : snapshotKind(x.latest).title, when: ago(x.latest.timestamp) }) : $t('game.branches.noSnapshots')}</div>
      </div>
      <div class="fact devices" title={x.on.some((d) => !d.here) ? $t('game.branches.asOfSync') : ''}>
        <div class="cap">{$t('game.branches.devicesOn')}</div>
        <div class="val">{deviceNames(x.on)}</div>
      </div>
      <div class="acts">
        {#if !active}
          <button class="btn small" disabled={$busy} on:click={() => switchBranch(x.b.name)}>{$t('game.branches.switchTo')}</button>
        {/if}
        {#if !active && x.b.name !== 'main'}
          <span class="more-wrap">
            <button class="more" class:on={menuFor === x.b.name} aria-label={$t('game.branches.more', { name: x.b.name })} aria-haspopup="menu" aria-expanded={menuFor === x.b.name} on:click={() => (menuFor = menuFor === x.b.name ? null : x.b.name)}><MoreHorizontal size={17} /></button>
            {#if menuFor === x.b.name}
              <Dropdown items={[{ label: $t('common.delete'), icon: Trash2, danger: true, run: () => deleteBranch(x.b.name) }]} open={true} label={$t('game.branches.more', { name: x.b.name })} on:close={() => (menuFor = null)} />
            {/if}
          </span>
        {/if}
      </div>
    </div>
  {/each}
</div>
<p class="footnote">{$t('game.branches.footnote')}</p>

{#if dialogOpen}
  <NewBranchDialog activeBranch={game.activeBranch} taken={Object.keys(game.branches ?? {})} busy={$busy} on:create={(e) => createBranch(e.detail)} on:cancel={() => (dialogOpen = false)} />
{/if}

<style>
  .intro {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 12px;
  }
  .intro p {
    flex: 1;
    font-size: var(--text-md);
    color: var(--text-dim);
  }
  .list {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .branch {
    display: flex;
    align-items: center;
    gap: 16px;
    padding: 12px 14px 12px 16px;
    border-radius: var(--radius-lg);
    border: 1px solid var(--border);
    background: var(--bg-raised);
    animation: arrive 0.34s cubic-bezier(0.2, 0.7, 0.2, 1) backwards;
    animation-delay: calc(var(--i) * 45ms);
    transition:
      border-color 0.4s,
      background 0.4s;
  }
  /* The one being played, in the game's own colour. */
  .branch.active {
    border-color: rgba(var(--game-accent-rgb, var(--accent-rgb)), 0.45);
    background: linear-gradient(0deg, rgba(var(--game-accent-rgb, var(--accent-rgb)), 0.1), rgba(var(--game-accent-rgb, var(--accent-rgb)), 0.1)), var(--bg-raised);
  }
  .btile {
    width: 38px;
    height: 38px;
    flex: none;
    border-radius: 10px;
    display: grid;
    place-items: center;
    background: var(--bg-active);
    color: var(--text-dim);
    transition:
      background 0.4s,
      color 0.4s;
  }
  .active .btile {
    background: rgba(var(--game-accent-rgb, var(--accent-rgb)), 0.2);
    color: var(--game-text, var(--accent-text));
  }
  .main {
    flex: 1;
    min-width: 0;
  }
  .name-row {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }
  .name {
    font-weight: 600;
    font-size: var(--text-base);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .real {
    font-size: var(--text-xs);
    color: var(--text-faint);
  }
  .chip {
    flex: none;
    padding: 1px 9px;
    border-radius: 999px;
    border: 1px solid rgba(var(--game-accent-rgb, var(--accent-rgb)), 0.5);
    color: var(--game-text, var(--accent-text));
    font-size: var(--text-xs);
    font-weight: 600;
  }
  .origin {
    font-size: var(--text-md);
    color: var(--text-dim);
  }
  .fact {
    flex: none;
    min-width: 0;
  }
  .latest {
    width: 220px;
  }
  .devices {
    width: 170px;
  }
  .cap {
    font-size: var(--text-xs);
    color: var(--text-faint);
  }
  .val {
    font-size: var(--text-md);
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .acts {
    flex: none;
    width: 150px;
    display: flex;
    justify-content: flex-end;
    align-items: center;
    gap: 6px;
  }
  .more-wrap {
    position: relative;
  }
  .more {
    width: 32px;
    height: 32px;
    display: grid;
    place-items: center;
    border: 0;
    border-radius: 8px;
    background: transparent;
    color: var(--text-faint);
    cursor: pointer;
  }
  .more:hover,
  .more.on {
    background: var(--bg-active);
    color: var(--text);
  }
  .footnote {
    margin-top: 12px;
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
  /* Narrow: the facts go under the name. */
  @media (max-width: 1100px) {
    .branch {
      flex-wrap: wrap;
    }
    .main {
      flex-basis: calc(100% - 60px);
    }
    /* Same columns on every card, so they line up down the list. */
    .latest {
      width: calc(50% - 60px);
      margin-left: 54px;
    }
    .devices {
      width: calc(50% - 60px);
    }
    .acts {
      flex: 1;
      width: auto;
    }
  }
</style>
