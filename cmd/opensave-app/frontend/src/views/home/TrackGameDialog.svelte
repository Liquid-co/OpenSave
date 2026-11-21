<script>
  // Tracking a game by hand, as the game database knows it: one running now,
  // one found by name, or the one a program or folder belongs to. Whatever it
  // is picked by,
  // it is tracked with its name, App ID (cover art, launching) and a save
  // location the database knows on this device — what a folder tracked on its
  // own lacked. A game tracked already under another entry is offered to link
  // that entry instead. Fires `close` on Cancel and after tracking, which also
  // opens the game's page.
  import { createEventDispatcher, onDestroy } from 'svelte';
  import Gamepad2 from 'lucide-svelte/icons/gamepad-2';
  import Folder from 'lucide-svelte/icons/folder';
  import FolderOpen from 'lucide-svelte/icons/folder-open';
  import AppWindow from 'lucide-svelte/icons/app-window';
  import Search from 'lucide-svelte/icons/search';
  import ArrowLeft from 'lucide-svelte/icons/arrow-left';
  import Play from 'lucide-svelte/icons/play';
  import { rise } from '../../lib/motion.js';
  import Modal from '../../components/ui/Modal.svelte';
  import ModalFoot from '../../components/ui/ModalFoot.svelte';
  import Spinner from '../../components/ui/Spinner.svelte';
  import { navigate, toast } from '../../lib/stores.js';
  import { api, native, coverURL } from '../../lib/api.js';
  import { t } from '../../lib/i18n.js';

  const dispatch = createEventDispatcher();
  const canPick = native.isWails();

  let query = '';
  let results = [];
  let searching = false;
  let note = '';

  // The games running now that the game database knows: the game being
  // played is the one most often tracked. null while being looked for.
  let running = null;
  (async () => {
    try {
      running = (await api.get('/api/gamedb/running')).games ?? [];
    } catch {
      running = [];
    }
  })();

  // The game picked, and how it is to be tracked.
  let picked = null;
  let name = '';
  let savePath = '';
  let exePath = '';
  let adding = false;

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
    note = $t('home.findGame.notKnown');
  }

  async function pickProgram() {
    const exe = await native.selectExecutable($t('home.findGame.programDialog'));
    if (exe) await identify(exe, true);
  }

  async function pickFolder() {
    const dir = await native.selectDirectory($t('home.findGame.saveFolderDialog'));
    if (dir) await identify(dir, false);
  }

  async function pickSaveFolder() {
    const dir = await native.selectDirectory($t('home.findGame.saveFolderOf', { name: name || $t('home.findGame.theGame') }));
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
      toast($t('home.findGame.linked', { tracked: picked.trackedName, name: picked.name }), 'success');
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
      toast($t('common.nowTracking', { name: game.name ?? name }), 'success', { game });
      dispatch('close');
      navigate('game', { gameId: game.id });
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      adding = false;
    }
  }
</script>

<Modal title={$t('home.findGame.title')} icon={Gamepad2} width={620} height="auto" onClose={() => dispatch('close')}>
  <div class="body">
    {#if !picked}
      <div class="field">
        <span class="label">{$t('home.findGame.runningNow')}</span>
        {#if running === null}
          <p class="hint"><Spinner size={13} /> {$t('home.findGame.lookingAtRunning')}</p>
        {:else if running.length === 0}
          <p class="hint">{$t('home.findGame.nothingRunning')}</p>
        {:else}
          <div class="list">
            {#each running as g, i (g.exe)}
              <button class="row" style="--i: {i}" on:click={() => choose(g, g.exe)}>
                <span class="art live"><Play size={14} /></span>
                <span class="row-text">
                <span class="row-name">{g.name}</span>
                <span class="row-sub">
                  {g.trackedName ? $t('home.findGame.trackedAs', { name: g.trackedName }) : g.exe}
                </span>
                </span>
              </button>
            {/each}
          </div>
        {/if}
      </div>
      <div class="field">
        <label for="tg-search">{$t('home.findGame.findByName')}</label>
        <div class="find">
          <Search size={15} />
          <input id="tg-search" placeholder={$t('home.findGame.searchPlaceholder')} bind:value={query} />
        </div>
        {#if searching}
          <p class="hint"><Spinner size={13} /> {$t('home.findGame.searching')}</p>
        {:else if results.length > 0}
          <div class="list">
            {#each results as g, i (g.name)}
              <button class="row" style="--i: {i}" on:click={() => choose(g)}>
                <span class="art">
                  {#if g.appId}<img src={coverURL(g.appId, true, g.name)} alt="" loading="lazy" on:error={(e) => (e.currentTarget.style.display = 'none')} />{/if}
                </span>
                <span class="row-text">
                <span class="row-name">{g.name}</span>
                <span class="row-sub">
                  {g.trackedName
                    ? $t('home.findGame.trackedAs', { name: g.trackedName })
                    : g.savePaths?.length
                      ? g.savePaths[0]
                      : $t('home.findGame.noSavesHere')}
                </span>
                </span>
              </button>
            {/each}
          </div>
        {:else if query.trim().length >= 2}
          <p class="hint">{$t('home.findGame.nothingFound')}</p>
        {/if}
      </div>

      {#if canPick}
        <div class="alt">
          <button class="mini" on:click={pickProgram}><AppWindow size={14} />{$t('home.findGame.chooseProgram')}</button>
          <button class="mini" on:click={pickFolder}><FolderOpen size={14} />{$t('home.findGame.chooseSaveFolder')}</button>
        </div>
      {/if}
    {:else}
      <div class="chosen" in:rise={{ y: 6 }}>
        {#if picked.appId}
          <img class="cover" src={coverURL(picked.appId)} alt="" on:error={(e) => (e.currentTarget.style.display = 'none')} />
        {/if}
        <div class="field grow">
          <label for="tg-name">{$t('home.addGame.name')}</label>
          <input id="tg-name" bind:value={name} placeholder={$t('home.addGame.namePlaceholder')} />
        </div>
      </div>
      {#if note}<p class="hint">{note}</p>{/if}
      {#if picked.trackedId}
        <div class="already">
          <p class="hint">{$t('home.findGame.already', { tracked: picked.trackedName, name: picked.name || $t('home.findGame.theGame') })}</p>
          <button class="btn" disabled={adding} on:click={link}>
            {$t('home.findGame.link', { tracked: picked.trackedName, name: picked.name })}
          </button>
        </div>
      {/if}

      <div class="field">
        <span class="label">{$t('home.findGame.saveFolder')}</span>
        {#if picked.savePaths?.length > 1}
          {#each picked.savePaths as p}
            <label class="check"><input type="radio" bind:group={savePath} value={p} /> <bdi>{p}</bdi></label>
          {/each}
        {:else if savePath}
          <div class="folder"><span class="ftile"><Folder size={18} /></span><div class="picked" title={savePath}><bdi>{savePath}</bdi></div></div>
        {:else}
          <p class="hint">{$t('home.findGame.noSavesYet')}</p>
        {/if}
        {#if canPick}
          <div class="alt"><button class="mini" on:click={pickSaveFolder}><FolderOpen size={14} />{$t('home.findGame.otherSaveFolder')}</button></div>
        {/if}
      </div>
      {#if exePath}
        <div class="field">
          <span class="label">{$t('home.findGame.program')}</span>
          <div class="folder"><span class="ftile"><AppWindow size={18} /></span><div class="picked" title={exePath}><bdi>{exePath}</bdi></div></div>
        </div>
      {/if}
      <div class="alt"><button class="mini" on:click={() => (picked = null)}><ArrowLeft size={14} />{$t('home.findGame.pickAnother')}</button></div>
    {/if}
  </div>
  <ModalFoot>
    <span />
    <div class="actions">
      <button class="btn" on:click={() => dispatch('close')}>{$t('common.cancel')}</button>
      <button class="btn primary" disabled={!picked || picked.trackedId || !name.trim() || !savePath || adding} on:click={track}>
        {adding ? $t('home.addGame.adding') : $t('home.addGame.startTracking')}
      </button>
    </div>
  </ModalFoot>
</Modal>

<style>
  .body {
    padding: 20px 22px 22px;
    display: flex;
    flex-direction: column;
    gap: 18px;
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: 7px;
    margin: 0;
  }
  .grow {
    flex: 1;
  }
  .label,
  label {
    font-size: var(--text-md);
    font-weight: 600;
    color: var(--text-dim);
  }
  /* A folder or program picked: its icon, then the path, the end of which
     names the game, so a long one keeps its end. */
  .folder {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 8px 12px 8px 8px;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: calc(12px * var(--round, 1));
    min-width: 0;
  }
  .ftile {
    width: 34px;
    height: 34px;
    flex-shrink: 0;
    border-radius: calc(9px * var(--round, 1));
    display: grid;
    place-items: center;
    background: var(--accent-soft);
    color: var(--accent-text);
  }
  .picked {
    flex: 1;
    min-width: 0;
    font-family: var(--font-mono, ui-monospace, monospace);
    font-size: var(--text-sm);
    color: var(--text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    direction: rtl;
    text-align: left;
  }
  .alt {
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
  }
  .mini {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: 28px;
    padding: 0 10px;
    border: 1px solid var(--border);
    border-radius: calc(8px * var(--round, 1));
    background: transparent;
    color: var(--text-dim);
    font: inherit;
    font-size: var(--text-sm);
    cursor: pointer;
    transition:
      background 0.12s,
      color 0.12s,
      border-color 0.12s;
  }
  .mini:hover {
    background: var(--bg-hover);
    border-color: var(--border-strong);
    color: var(--text);
  }
  .hint {
    margin: 0;
  }
  .actions {
    display: flex;
    gap: 8px;
  }
  .find {
    display: flex;
    align-items: center;
    gap: 9px;
    height: 38px;
    padding: 0 12px;
    background: var(--bg);
    border: 1px solid var(--border-strong);
    border-radius: calc(10px * var(--round, 1));
    color: var(--text-faint);
    transition:
      border-color 0.12s,
      box-shadow 0.12s;
  }
  .find:focus-within {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px rgba(var(--accent-rgb), 0.16);
  }
  .find input {
    flex: 1;
    min-width: 0;
    border: none;
    outline: none;
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: var(--text-base);
    min-height: 0;
    padding: 0;
    box-shadow: none;
  }
  .list {
    display: flex;
    flex-direction: column;
    gap: 2px;
    max-height: 240px;
    overflow-y: auto;
    padding: 4px;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: calc(12px * var(--round, 1));
  }
  .row {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 6px 10px 6px 6px;
    background: none;
    border: none;
    border-radius: calc(9px * var(--round, 1));
    color: var(--text);
    font: inherit;
    text-align: left;
    cursor: pointer;
    flex-shrink: 0;
    transition: background 0.12s;
    animation: row-in 0.22s cubic-bezier(0.2, 0.7, 0.2, 1) backwards;
    animation-delay: calc(min(var(--i), 8) * 25ms);
  }
  @keyframes row-in {
    from {
      opacity: 0;
      transform: translateY(4px);
    }
  }
  .row:hover:not(:disabled),
  .row:focus-visible {
    background: var(--bg-hover);
  }
  .row:disabled {
    opacity: 0.5;
    cursor: default;
  }
  .art {
    position: relative;
    width: 30px;
    height: 40px;
    flex-shrink: 0;
    border-radius: calc(5px * var(--round, 1));
    overflow: hidden;
    background: var(--bg-active);
  }
  .art img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .art.live {
    display: grid;
    place-items: center;
    color: var(--success);
    background: rgba(var(--success-rgb), 0.12);
  }
  .row-text {
    display: flex;
    flex-direction: column;
    gap: 1px;
    min-width: 0;
  }
  .row-name {
    font-weight: 600;
  }
  .row-sub {
    font-size: var(--text-sm);
    color: var(--text-faint);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 100%;
  }
  .chosen {
    display: flex;
    gap: 16px;
    align-items: flex-end;
  }
  .cover {
    width: 150px;
    aspect-ratio: 460 / 215;
    object-fit: cover;
    border-radius: calc(10px * var(--round, 1));
    box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.08), var(--shadow-lift);
  }
  /* A save folder to choose among several: a radio row, not a field label. */
  label.check {
    font-weight: 400;
    font-size: var(--text-md);
    color: var(--text);
  }
  .already {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 10px;
    padding: 12px 14px;
    border: 1px solid rgba(var(--warn-rgb), 0.3);
    background: rgba(var(--warn-rgb), 0.08);
    border-radius: calc(12px * var(--round, 1));
  }
</style>
