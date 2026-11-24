<script>
  // Branches: separate lines of saves for one game — a second playthrough, or
  // a run kept apart from the main one.
  import { askConfirm } from '../../lib/stores.js';
  import { api } from '../../lib/api.js';
  import NewBranchDialog from './NewBranchDialog.svelte';
  import { t } from '../../lib/i18n.js';
  import Plus from 'lucide-svelte/icons/plus';

  export let game;
  export let runner;
  const { busy, run } = runner;

  $: branches = Object.values(game.branches ?? {});

  let newBranch = '';
  // Creating a branch asks what it starts from in its own dialog rather than
  // a checkbox beside the name field. The two answers do materially
  // different things to the save folder on the next switch, which is not a
  // decision to make by noticing a tickbox.
  let dialogOpen = false;
  function openDialog() {
    if (!newBranch || $busy) return;
    dialogOpen = true;
  }
  const createBranch = (copyCurrentSave) => {
    dialogOpen = false;
    return run(copyCurrentSave ? $t('game.branches.createdFromCurrent') : $t('game.branches.createdEmpty'), async () => {
      await api.post(`/api/games/${game.id}/branch`, { name: newBranch, copyCurrentSave });
      newBranch = '';
    });
  };
  const switchBranch = (name) =>
    run($t('game.branches.switchedTo', { name }), () => api.post(`/api/games/${game.id}/branch/switch`, { name }));
  async function deleteBranch(name) {
    if (!(await askConfirm($t('game.branches.deleteConfirm', { name }), { title: $t('game.branches.deleteTitle'), confirmText: $t('common.delete'), danger: true }))) return;
    run($t('game.branches.deleted', { name }), () => api.del(`/api/games/${game.id}/branch/${encodeURIComponent(name)}`));
  }
</script>

<div class="card branch-new">
  <div class="new-row">
    <input
      placeholder={$t('game.branches.namePlaceholder')}
      bind:value={newBranch}
      on:keydown={(e) => e.key === 'Enter' && openDialog()}
    />
    <button class="btn primary" disabled={!newBranch || $busy} on:click={openDialog}><Plus size={15} />{$t('game.branches.create')}</button>
  </div>
  <span class="hint">
    {$t('game.branches.hint')}
  </span>
</div>
<div class="list">
  {#each branches as branch (branch.name)}
    <div class="card row">
      <div class="info">
        <div class="top">
          <span class="name">{branch.name}</span>
          {#if branch.name === game.activeBranch}<span class="badge online">{$t('game.branches.active')}</span>{/if}
        </div>
        <div class="meta">{$t('game.branches.snapshotCount', { n: branch.snapshots?.length ?? 0 })}</div>
      </div>
      {#if branch.name !== game.activeBranch}
        <div class="actions">
          <button class="btn small primary" disabled={$busy} on:click={() => switchBranch(branch.name)}>
            {$t('game.branches.switchTo')}
          </button>
          {#if branch.name !== 'main'}
            <button class="btn small danger" disabled={$busy} on:click={() => deleteBranch(branch.name)}>
              {$t('common.delete')}
            </button>
          {/if}
        </div>
      {/if}
    </div>
  {/each}
</div>
<p class="footnote">
  {$t('game.branches.footnote')}
</p>

{#if dialogOpen}
  <NewBranchDialog
    name={newBranch}
    activeBranch={game.activeBranch}
    busy={$busy}
    on:create={(e) => createBranch(e.detail)}
    on:cancel={() => (dialogOpen = false)}
  />
{/if}

<style>
  .branch-new {
    padding: 14px;
    margin-bottom: 14px;
  }
  .new-row {
    display: flex;
    gap: 10px;
  }
  .new-row input {
    flex: 1;
    padding: 8px 12px;
    background: var(--bg);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius);
    color: var(--text);
    outline: none;
  }
  .branch-new .hint {
    display: block;
    margin-top: 10px;
  }
  .list {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .row {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 14px 16px;
  }
  .info {
    flex: 1;
    min-width: 0;
  }
  .top {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 4px;
  }
  .name {
    font-weight: 600;
    font-size: 0.9rem;
  }
  .meta {
    font-size: 0.75rem;
    color: var(--text-faint);
  }
  .actions {
    display: flex;
    gap: 6px;
  }
  .footnote {
    margin-top: 12px;
    font-size: 0.8rem;
    color: var(--text-faint);
  }
</style>
