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
    const exe = await native.selectFile($t('home.findGame.programDialog'));
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
      toast($t('common.nowTracking', { name: game.name ?? name }), 'success');
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
            {#each running as g (g.exe)}
              <button class="row" on:click={() => choose(g, g.exe)}>
                <span class="row-name">{g.name}</span>
                <span class="row-sub">
                  {g.trackedName ? $t('home.findGame.trackedAs', { name: g.trackedName }) : g.exe}
                </span>
              </button>
            {/each}
          </div>
        {/if}
      </div>
      <div class="field">
        <label for="tg-search">{$t('home.findGame.findByName')}</label>
        <input id="tg-search" placeholder={$t('home.findGame.searchPlaceholder')} bind:value={query} />
        {#if searching}
          <p class="hint"><Spinner size={13} /> {$t('home.findGame.searching')}</p>
        {:else if results.length > 0}
          <div class="list">
            {#each results as g (g.name)}
              <button class="row" on:click={() => choose(g)}>
                <span class="row-name">{g.name}</span>
                <span class="row-sub">
                  {g.trackedName
                    ? $t('home.findGame.trackedAs', { name: g.trackedName })
                    : g.savePaths?.length
                      ? g.savePaths[0]
                      : $t('home.findGame.noSavesHere')}
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
          <button class="linklike" on:click={pickProgram}>{$t('home.findGame.chooseProgram')}</button>
          <button class="linklike" on:click={pickFolder}>{$t('home.findGame.chooseSaveFolder')}</button>
        </div>
      {/if}
    {:else}
      <div class="chosen">
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
          <div class="picked" title={savePath}><bdi>{savePath}</bdi></div>
        {:else}
          <p class="hint">{$t('home.findGame.noSavesYet')}</p>
        {/if}
        {#if canPick}
          <div class="alt"><button class="linklike" on:click={pickSaveFolder}>{$t('home.findGame.otherSaveFolder')}</button></div>
        {/if}
      </div>
      {#if exePath}
        <div class="field">
          <span class="label">{$t('home.findGame.program')}</span>
          <div class="picked" title={exePath}><bdi>{exePath}</bdi></div>
        </div>
      {/if}
      <div class="alt"><button class="linklike" on:click={() => (picked = null)}>{$t('home.findGame.pickAnother')}</button></div>
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
