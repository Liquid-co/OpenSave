<script>
  // Tracking a game by hand, as the game database knows it: one running now,
  // one found by name, or the one a program or folder belongs to. Whatever it
  // is picked by, it is tracked with its name, App ID (cover art, launching,
  // seeing it played) and a save location the database knows on this device —
  // what a folder tracked on its own lacked. Fires `close` on Cancel and after
  // tracking, which also opens the new game's page.
  import { createEventDispatcher, onMount, onDestroy } from 'svelte';
  import Gamepad2 from 'lucide-svelte/icons/gamepad-2';
  import Modal from '../../components/ui/Modal.svelte';
  import ModalFoot from '../../components/ui/ModalFoot.svelte';
  import Spinner from '../../components/ui/Spinner.svelte';
  import { navigate, toast } from '../../lib/stores.js';
  import { api, native, coverURL } from '../../lib/api.js';

  const dispatch = createEventDispatcher();
  const canPick = native.isWails();

  let running = null; // null while being looked for
  let query = '';
  let results = [];
  let searching = false;
  let note = '';

  // The game picked, and how it is to be tracked.
  let picked = null;
  let name = '';
  let savePath = '';
  let exePath = '';
  let adding = false;

  onMount(async () => {
    try {
      running = (await api.get('/api/gamedb/running')).games ?? [];
    } catch {
      running = [];
    }
  });

  let searchTimer = null;
  onDestroy(() => clearTimeout(searchTimer));
  $: queueSearch(query);
  function queueSearch(q) {
    clearTimeout(searchTimer);
    if ((q ?? '').trim().length < 2) {
      results = [];
      return;
    }
    searchTimer = setTimeout(async () => {
      searching = true;
      try {
        results = (await api.get(`/api/gamedb/search?q=${encodeURIComponent(q.trim())}`)).games ?? [];
      } catch {
        results = [];
      } finally {
        searching = false;
      }
    }, 250);
  }

  function choose(game, exe = '') {
    picked = game;
    name = game.name;
    savePath = game.savePaths?.[0] ?? '';
    exePath = exe;
    note = '';
  }

  async function identify(path, exe) {
    note = '';
    try {
      const res = await api.get(`/api/gamedb/identify?path=${encodeURIComponent(path)}`);
      if (res.found) {
        choose(res.game, exe ? path : '');
        if (!exe && !savePath) savePath = path;
        return;
      }
    } catch {
      // Not known: tracked as picked, under a name to give it.
    }
    picked = { name: '', appId: '', savePaths: [] };
    name = '';
    exePath = exe ? path : '';
    savePath = exe ? '' : path;
    note = 'Not in the game database — give it a name, and pick its save folder if it is not set.';
  }

  async function pickProgram() {
    const exe = await native.selectFile('Choose the game’s program');
    if (exe) await identify(exe, true);
  }

  async function pickFolder() {
    const dir = await native.selectDirectory('Choose the game’s save folder');
    if (dir) await identify(dir, false);
  }

  async function pickSaveFolder() {
    const dir = await native.selectDirectory(`Save folder of “${name || 'the game'}”`);
    if (dir) savePath = dir;
  }

  // The game is tracked already under another entry: give that one what the
  // game database knows instead of tracking the same folder twice.
  async function link() {
    if (!picked?.trackedId || adding) return;
    adding = true;
    try {
      const body = { name: picked.name };
      if (picked.appId) body.appId = picked.appId;
      if (exePath) body.exePath = exePath;
      await api.patch(`/api/games/${picked.trackedId}`, body);
      toast(`“${picked.trackedName}” is now ${picked.name}`, 'success');
      dispatch('close');
      navigate('game', { gameId: picked.trackedId });
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      adding = false;
    }
  }

  async function track() {
    if (!name.trim() || !savePath || adding) return;
    adding = true;
    try {
      const game = await api.post('/api/games', {
        name: name.trim(),
        savePath,
        appId: picked?.appId ?? '',
        exePath
      });
      toast(`Now tracking "${game.name ?? name}"`, 'success');
      dispatch('close');
      navigate('game', { gameId: game.id });
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      adding = false;
    }
  }
</script>

<Modal title="Track a game" icon={Gamepad2} width={620} height="auto" onClose={() => dispatch('close')}>
  <div class="body">
    {#if !picked}
      <div class="field">
        <span class="label">Running now</span>
        {#if running === null}
          <p class="hint"><Spinner size={13} /> Looking at what is running…</p>
        {:else if running.length === 0}
          <p class="hint">No game the game database knows is running right now.</p>
        {:else}
          <div class="list">
            {#each running as g (g.exe)}
              <button class="row" disabled={g.tracked && !g.trackedId} on:click={() => choose(g, g.exe)}>
                <span class="row-name">{g.name}</span>
                <span class="row-sub">{g.trackedName ? `tracked as “${g.trackedName}”` : g.tracked ? 'already tracked' : g.installDir || g.exe}</span>
              </button>
            {/each}
          </div>
        {/if}
      </div>

      <div class="field">
        <label for="tg-search">Find by name</label>
        <input id="tg-search" placeholder="e.g. Crimson Desert" bind:value={query} />
        {#if searching}
          <p class="hint"><Spinner size={13} /> Searching…</p>
        {:else if results.length > 0}
          <div class="list">
            {#each results as g (g.name)}
              <button class="row" on:click={() => choose(g)}>
                <span class="row-name">{g.name}</span>
                <span class="row-sub">
                  {g.trackedName ? `tracked as “${g.trackedName}”` : g.savePaths?.length ? g.savePaths[0] : 'no saves found on this PC'}
                </span>
              </button>
            {/each}
          </div>
        {:else if query.trim().length >= 2}
          <p class="hint">Nothing by that name in the game database.</p>
        {/if}
      </div>

      {#if canPick}
        <div class="alt">
          <button class="linklike" on:click={pickProgram}>Choose the game’s program…</button>
          <button class="linklike" on:click={pickFolder}>Choose its save folder…</button>
        </div>
      {/if}
    {:else}
      <div class="chosen">
        {#if picked.appId}
          <img class="cover" src={coverURL(picked.appId)} alt="" on:error={(e) => (e.currentTarget.style.display = 'none')} />
        {/if}
        <div class="field grow">
          <label for="tg-name">Name</label>
          <input id="tg-name" bind:value={name} placeholder="e.g. Elden Ring" />
        </div>
      </div>
      {#if note}<p class="hint">{note}</p>{/if}
      {#if picked.trackedId}
        <div class="already">
          <p class="hint">
            Already tracked here as <strong>“{picked.trackedName}”</strong>, at the same save folder. Tracked again it would
            sync the same files twice; link that entry to {picked.name || 'this game'} instead — its name, cover art,
            settings and seeing it played come from the game database.
          </p>
          <button class="btn" disabled={adding} on:click={link}>Link “{picked.trackedName}” to {picked.name}</button>
        </div>
      {/if}

      <div class="field">
        <span class="label">Save folder</span>
        {#if picked.savePaths?.length > 1}
          {#each picked.savePaths as p}
            <label class="check"><input type="radio" bind:group={savePath} value={p} /> <bdi>{p}</bdi></label>
          {/each}
        {:else if savePath}
          <div class="picked" title={savePath}><bdi>{savePath}</bdi></div>
        {:else}
          <p class="hint">No saves of it found on this PC yet. Choose where it saves, or play it once and track it then.</p>
        {/if}
        {#if canPick}
          <div class="alt"><button class="linklike" on:click={pickSaveFolder}>Choose another save folder…</button></div>
        {/if}
      </div>
      {#if exePath}
        <div class="field">
          <span class="label">Program</span>
          <div class="picked" title={exePath}><bdi>{exePath}</bdi></div>
        </div>
      {/if}
      <div class="alt"><button class="linklike" on:click={() => (picked = null)}>← Pick another game</button></div>
    {/if}
  </div>
  <ModalFoot>
    <span />
    <div class="actions">
      <button class="btn" on:click={() => dispatch('close')}>Cancel</button>
      <button class="btn primary" disabled={!picked || picked.trackedId || !name.trim() || !savePath || adding} on:click={track}>
        {adding ? 'Adding…' : 'Start tracking'}
      </button>
    </div>
  </ModalFoot>
</Modal>

<style>
  .body {
    padding: 18px 22px 20px;
    display: flex;
    flex-direction: column;
    gap: 16px;
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .grow {
    flex: 1;
  }
  .label,
  label {
    font-size: 0.82rem;
    color: var(--text-dim);
  }
  .list {
    display: flex;
    flex-direction: column;
    max-height: 220px;
    overflow-y: auto;
    border: 1px solid var(--border);
    border-radius: 8px;
  }
  .row {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 2px;
    padding: 8px 10px;
    background: none;
    border: none;
    border-bottom: 1px solid var(--border);
    color: var(--text);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .row:last-child {
    border-bottom: none;
  }
  .row:hover:not(:disabled) {
    background: var(--bg);
  }
  .row:disabled {
    opacity: 0.5;
    cursor: default;
  }
  .row-sub {
    font-size: 0.78rem;
    color: var(--text-dim);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 100%;
  }
  .chosen {
    display: flex;
    gap: 14px;
    align-items: flex-end;
  }
  .cover {
    width: 140px;
    aspect-ratio: 460 / 215;
    object-fit: cover;
    border-radius: 8px;
    border: 1px solid var(--border);
  }
  .picked {
    font-family: var(--font-mono, ui-monospace, monospace);
    font-size: 0.8rem;
    color: var(--text);
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 8px 10px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    direction: rtl;
    text-align: left;
  }
  .alt {
    display: flex;
    gap: 16px;
  }
  .linklike {
    background: none;
    border: none;
    padding: 0;
    font: inherit;
    font-size: 0.8rem;
    color: var(--text-dim);
    text-decoration: underline;
    cursor: pointer;
  }
  .linklike:hover {
    color: var(--text);
  }
  .hint {
    margin: 0;
  }
  .already {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 8px;
    padding: 10px 12px;
    border: 1px solid var(--border);
    border-radius: 8px;
  }
  .actions {
    display: flex;
    gap: 8px;
  }
</style>
