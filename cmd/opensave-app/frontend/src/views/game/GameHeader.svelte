<script>
  // The top of a game's page: its art and name, which devices have this save,
  // the Launch and Sync buttons, and the save folder with a way to open it.
  import { onDestroy } from 'svelte';
  import { peers, navigate, toast, syncActivity } from '../../lib/stores.js';
  import { api, native, gameCover } from '../../lib/api.js';
  import { timeAgo } from '../../lib/timeago.js';
  import ArrowLeft from 'lucide-svelte/icons/arrow-left';
  import Play from 'lucide-svelte/icons/play';
  import RefreshCw from 'lucide-svelte/icons/refresh-cw';
  import FolderOpen from 'lucide-svelte/icons/folder-open';
  import Pencil from 'lucide-svelte/icons/pencil';

  export let game;
  /** The page's shared runner (see lib/runner.js). */
  export let runner;
  const { busy, run } = runner;

  $: activity = $syncActivity[game.id];

  // Is my Deck up to date with THIS save? One entry per paired device, in
  // the header, because it is the question a person opens a game to ask.
  // "never" is listed too: it is the answer that explains why a save is not
  // on the other machine. A device that was unpaired is not shown even if a
  // stamp for it survived somewhere — the list is the paired devices.
  $: syncedWith = Object.values($peers)
    .map((p) => ({ id: p.id, name: p.name, at: game?.lastSyncedWith?.[p.id] ?? '' }))
    .sort((a, b) => a.name.localeCompare(b.name));
  // "4 min ago" must not freeze at the moment the page was opened.
  let now = Date.now();
  const tick = setInterval(() => (now = Date.now()), 30_000);
  onDestroy(() => clearInterval(tick));

  const syncNow = () => run('Sync triggered', () => api.post(`/api/games/${game.id}/sync`));
  const launchGame = () => run('Launching…', () => api.post(`/api/games/${game.id}/launch`));

  let editPath = false;
  let pathDraft = '';
  // Reveal the save location in Explorer / Finder / the Linux file manager.
  // The bridge returns a message when it can't (e.g. the folder was deleted).
  async function openSaveFolder() {
    const problem = await native.openFolder(game.savePath);
    if (problem) toast(problem, 'error');
  }

  async function savePath() {
    await run('Save path updated', () => api.patch(`/api/games/${game.id}`, { savePath: pathDraft }));
    editPath = false;
  }
</script>

<div class="head">
  <button class="btn icon back" on:click={() => navigate('home')} title="Back" aria-label="Back"><ArrowLeft size={18} /></button>
  {#if gameCover(game)}
    <img
      class="head-cover"
      src={gameCover(game)}
      alt=""
      on:load={(e) => (e.currentTarget.style.display = '')}
      on:error={(e) => (e.currentTarget.style.display = 'none')}
    />
  {/if}
  <div class="title-block">
    <h2 class="page-title">{game.name}</h2>
    <div class="sub">
      branch <strong>{game.activeBranch}</strong>
      {#if activity?.state === 'running'}
        · <span class="syncing">syncing {activity.percentage ?? 0}%</span>
      {/if}
    </div>
    {#if syncedWith.length > 0}
      <div class="sub synced-with">
        {#each syncedWith as d, i (d.id)}
          {#if i > 0}<span class="sep">·</span>{/if}
          <span class="device" title={d.at ? `${d.name}: last synced ${new Date(d.at).toLocaleString()}` : `${d.name} has never synced this game`}>
            {d.name}
            <span class:never={!d.at}>{d.at ? `synced ${timeAgo(d.at, now)}` : 'never synced'}</span>
          </span>
        {/each}
      </div>
    {/if}
  </div>
  <div class="head-actions">
    {#if game.appId || game.exePath}
      <button class="btn" disabled={$busy} on:click={launchGame}><Play size={15} />Launch</button>
    {/if}
    <button class="btn primary" disabled={$busy} on:click={syncNow}><RefreshCw size={15} />Sync now</button>
  </div>
</div>

<div class="path-line">
  {#if editPath}
    <input class="path-input" bind:value={pathDraft} />
    <button class="btn small" on:click={async () => (pathDraft = (await native.selectDirectory('Select save folder')) || pathDraft)}>Browse</button>
    <button class="btn small primary" on:click={savePath}>Save</button>
    <button class="btn small" on:click={() => (editPath = false)}>Cancel</button>
  {:else}
    <span class="path" title={game.savePath}>{game.savePath}</span>
    <button class="btn small" on:click={openSaveFolder} title="Show this folder in your file manager">
      <FolderOpen size={14} />Open folder
    </button>
    <button class="btn small" on:click={() => { pathDraft = game.savePath; editPath = true; }}><Pencil size={13} />Edit</button>
  {/if}
</div>

<style>
  .head {
    display: flex;
    align-items: center;
    gap: 14px;
    margin-bottom: 6px;
  }
  .back {
    font-size: 1rem;
  }
  .head-cover {
    height: 52px;
    aspect-ratio: 460 / 215;
    object-fit: cover;
    border-radius: 8px;
    border: 1px solid var(--border);
  }
  .title-block {
    flex: 1;
    min-width: 0;
  }
  .sub {
    color: var(--text-dim);
    font-size: 0.85rem;
    margin-top: 2px;
  }
  .syncing {
    color: var(--accent);
    font-weight: 600;
  }
  .synced-with {
    display: flex;
    flex-wrap: wrap;
    gap: 0 6px;
  }
  .synced-with .device {
    white-space: nowrap;
  }
  .synced-with .device > span {
    color: var(--text);
  }
  .synced-with .device > span.never {
    color: var(--text-dim);
    font-style: italic;
  }
  .synced-with .sep {
    color: var(--text-dim);
  }
  .head-actions {
    display: flex;
    gap: 8px;
  }
  .path-line {
    display: flex;
    align-items: center;
    gap: 8px;
    margin: 0 0 18px 50px;
  }
  .path {
    font-size: 0.8rem;
    color: var(--text-faint);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .path-input {
    flex: 1;
    padding: 6px 10px;
    background: var(--bg);
    border: 1px solid var(--border-strong);
    border-radius: 8px;
    color: var(--text);
    font-size: 0.82rem;
    outline: none;
  }
</style>
