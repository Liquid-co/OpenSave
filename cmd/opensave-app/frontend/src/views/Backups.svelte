<script>
  // Backups: your saves kept off this PC. A header over your own covers says
  // how many games have their current save in the cloud; below it every game
  // as its cover, with its copies stacked behind it. On the right, the backup
  // file, which is a feature of its own, and what the cloud does by itself.
  //
  // With nowhere to back up to yet, the page is the choice of where
  // (BackupsSetup). Everything shown comes from GET /api/cloud/overview
  // (internal/api/routes_cloud_overview.go) and is only what the app knows.
  //
  // Motion: the parts arrive one after another by moving, never by fading
  // (app.css "arrive"), so a slow frame never shows a blank page.
  import { onDestroy, onMount } from 'svelte';
  import { toast, settings, cloudUploadEvent, askConfirm, gameList, navigate } from '../lib/stores.js';
  import { openMenu } from '../lib/contextmenu.js';
  import { api } from '../lib/api.js';
  import { restoreKey } from '../lib/toasts.js';
  import { providerConfigured } from '../lib/cloudproviders.js';
  import { gameState, SHELF_FILTERS, inFilter } from '../lib/backupsview.js';
  import { t } from '../lib/i18n.js';
  import Skeleton from '../components/ui/Skeleton.svelte';
  import FilterTabs from '../components/ui/FilterTabs.svelte';
  import CloudOff from 'lucide-svelte/icons/cloud-off';
  import Layers from 'lucide-svelte/icons/layers';
  import ArrowUpRight from 'lucide-svelte/icons/arrow-up-right';
  import CloudUpload from 'lucide-svelte/icons/cloud-upload';
  import Trash2 from 'lucide-svelte/icons/trash-2';
  import BackupsHero from './backups/BackupsHero.svelte';
  import CoverShelf from './backups/CoverShelf.svelte';
  import CopiesDialog from './backups/CopiesDialog.svelte';
  import BackupFileCard from './backups/BackupFileCard.svelte';
  import AutoCard from './backups/AutoCard.svelte';
  import BackupsSetup from './backups/BackupsSetup.svelte';

  let config = null; // the cloud settings, null until read
  let autoPull = false;
  let setup = false; // choosing where, by choice rather than for want of a place

  async function loadConfig() {
    try {
      const s = await api.get('/api/settings');
      config = s.cloudSync ?? { enabled: false, provider: 'local', url: '', username: '', password: '', headers: '{}', folderId: '' };
      autoPull = !!s.cloudAutoPull;
    } catch (e) {
      toast(e.message, 'error');
    }
  }

  $: configured = !!config && providerConfigured(config);
  $: on = !!config?.enabled;
  // A web address takes copies but cannot list them back.
  $: listable = config?.provider !== 'webhook';

  // ── What the cloud holds ──
  let overview = null;
  let readError = '';
  let reading = false;
  async function read() {
    if (!configured || !on || !listable) return;
    reading = true;
    try {
      overview = await api.get('/api/cloud/overview');
      readError = '';
    } catch (e) {
      readError = e.message;
      // The daemon wipes dead tokens when the provider refuses them; read
      // the settings again so the page asks to sign in.
      if (/expired|reconnect|not authenticated|re-auth/i.test(e.message)) loadConfig();
    } finally {
      reading = false;
    }
  }
  $: if (configured && on && listable) read();

  // An upload finishing changes what is up there.
  let rereadTimer;
  const unsubUpload = cloudUploadEvent.subscribe((ev) => {
    if (!ev?.complete) return;
    clearTimeout(rereadTimer);
    rereadTimer = setTimeout(read, 600);
  });
  onDestroy(() => {
    unsubUpload();
    clearTimeout(rereadTimer);
  });
  onMount(loadConfig);

  let filter = 'all';
  $: games = overview ?? [];
  $: counts = Object.fromEntries(SHELF_FILTERS.map((f) => [f, games.filter((g) => inFilter(g, f)).length]));
  $: filterOptions = SHELF_FILTERS.map((f) => [f, $t('backups.filter.' + f)]);

  // ── Acting on it ──
  // { gameId, name, done, total } while Back up now or Upload now runs.
  let uploading = null;
  async function upload(list) {
    if (uploading || !list.length) return;
    uploading = { gameId: list[0].gameId, name: list[0].gameName, done: 0, total: list.length };
    let sent = 0;
    try {
      for (const g of list) {
        uploading = { ...uploading, gameId: g.gameId, name: g.gameName };
        const res = await api.post(`/api/cloud/sync-local/${g.gameId}`);
        sent += res.uploaded ?? 0;
        uploading = { ...uploading, done: uploading.done + 1 };
        // Each game shows as safe the moment its own upload is done.
        await read();
      }
      toast(sent ? $t('backups.uploaded', { n: sent }) : $t('backups.upToDate'), 'success');
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      uploading = null;
      read();
    }
  }
  function backUpNow() {
    const tracked = games.filter((g) => g.tracked);
    const behind = tracked.filter((g) => ['waiting', 'behind'].includes(gameState(g)));
    upload(behind.length ? behind : tracked);
  }

  async function putBack(g, c) {
    const when = new Date(c.takenMs || Date.parse(c.createdTime)).toLocaleString(undefined, { day: 'numeric', month: 'short', hour: 'numeric', minute: '2-digit' });
    if (!(await askConfirm($t('backups.putBackBody', { game: g.gameName, when }), { title: $t('backups.putBackTitle', { game: g.gameName }), confirmText: $t('backups.putBack') }))) return;
    try {
      await api.post(`/api/cloud/restore/${g.gameId}`, { fileName: c.name });
      toast($t('backups.putBackDone', { game: g.gameName }), 'success', { key: restoreKey(g.gameId), game: g.gameId });
    } catch (e) {
      toast(e.message, 'error', { key: restoreKey(g.gameId), game: g.gameId });
    }
  }

  // ── A cover's right-click menu ──
  // The game's copies, the game itself, an upload, and removing every copy
  // the cover stands for — the same copies the Copies dialog lists, removed
  // one at a time the way it removes one, so linked ids go too.
  let deletingId = '';
  function coverMenu({ event, game: g }) {
    const own = g.tracked && $gameList.some((x) => x.id === g.gameId);
    openMenu(event, [
      { label: $t('backups.menu.copies'), icon: Layers, run: () => (opened = g.gameId) },
      ...(own ? [{ label: $t('backups.menu.openGame'), icon: ArrowUpRight, run: () => navigate('game', { gameId: g.gameId }) }] : []),
      ...(own && g.localSnapshots > 0 ? [{ label: $t('backups.menu.backUp'), icon: CloudUpload, disabled: !!uploading, run: () => upload([g]) }] : []),
      null,
      { label: $t('backups.menu.delete'), icon: Trash2, danger: true, disabled: !g.copies.length || !!deletingId, run: () => deleteAll(g) }
    ]);
  }
  async function deleteAll(g) {
    const copies = [...g.copies];
    if (!copies.length || deletingId) return;
    const ok = await askConfirm($t('backups.deleteAllBody', { n: copies.length, game: g.gameName }), {
      title: $t('backups.deleteAllTitle', { game: g.gameName }),
      confirmText: $t('backups.deleteAllConfirm'),
      danger: true
    });
    if (!ok) return;
    deletingId = g.gameId;
    let done = 0;
    let error = '';
    try {
      for (const c of copies) {
        try {
          await api.post(`/api/cloud/delete/${g.gameId}`, { fileName: c.name, id: c.id ?? '' });
          done++;
        } catch (e) {
          error ||= e.message;
        }
      }
      if (error) toast($t('backups.deletePartial', { done, total: copies.length, game: g.gameName, error }), 'error');
      else toast($t('backups.deletedAll', { n: done, game: g.gameName }), 'success');
    } finally {
      deletingId = '';
      read();
    }
  }

  async function setEnabled(value) {
    const before = config.enabled;
    config = { ...config, enabled: value };
    try {
      settings.set(await api.post('/api/settings', { cloudSync: { enabled: value } }));
    } catch (e) {
      config = { ...config, enabled: before };
      toast($t('cloud.backup.saveFailedToast', { message: e.message }), 'error');
    }
  }
  async function setAutoPull(value) {
    const before = autoPull;
    autoPull = value;
    try {
      settings.set(await api.post('/api/settings', { cloudAutoPull: value }));
    } catch (e) {
      autoPull = before;
      toast($t('cloud.backup.saveFailedToast', { message: e.message }), 'error');
    }
  }

  let opened = null; // the game whose copies are open
  $: openedGame = opened ? games.find((g) => g.gameId === opened) : null;
  $: tracked = $gameList;
</script>

<div class="backups">
  {#if !config}
    <Skeleton kind="cards" count={2} />
  {:else if setup || !configured}
    <BackupsSetup
      {config}
      canGoBack={configured}
      on:back={() => (setup = false)}
      on:done={async () => {
        setup = false;
        await loadConfig();
        read();
      }}
    />
  {:else}
    <div class="cols">
      <div class="main">
        <div class="part" style="--n: 0">
          <BackupsHero
            {config}
            {games}
            {tracked}
            {on}
            {reading}
            {readError}
            {uploading}
            loaded={overview !== null}
            {listable}
            on:backup={backUpNow}
            on:retry={read}
            on:turnOn={() => setEnabled(true)}
            on:change={() => (setup = true)}
          />
        </div>

        <section class="shelf part" style="--n: 1" aria-labelledby="shelf-title">
          <div class="shelf-head">
            <h2 id="shelf-title">{$t('backups.inTheCloud')}</h2>
            {#if on && listable && overview}
              <FilterTabs options={filterOptions} {counts} bind:value={filter} />
            {/if}
          </div>

          {#if !on}
            <div class="quiet-box"><CloudOff size={22} /><p>{$t('backups.offList')}</p></div>
          {:else if !listable}
            <div class="quiet-box"><CloudOff size={22} /><p>{$t('backups.hero.webhookSub')}</p></div>
          {:else if readError && !overview}
            <div class="quiet-box"><CloudOff size={22} /><p>{$t('backups.readFailed')}</p></div>
          {:else if !overview}
            <Skeleton kind="cards" count={6} />
          {:else if !games.length}
            <div class="quiet-box"><p>{$t('backups.nothingTracked')}</p></div>
          {:else}
            <CoverShelf
              games={games.filter((g) => inFilter(g, filter))}
              {tracked}
              busy={!!uploading}
              uploadingId={uploading?.gameId ?? ''}
              on:open={(e) => (opened = e.detail)}
              on:upload={(e) => upload([e.detail])}
              on:menu={(e) => coverMenu(e.detail)}
            />
          {/if}
        </section>
      </div>

      <aside class="side">
        <div class="part" style="--n: 2"><BackupFileCard /></div>
        <div class="part" style="--n: 3"><AutoCard enabled={on} {autoPull} on:enabled={(e) => setEnabled(e.detail)} on:autoPull={(e) => setAutoPull(e.detail)} /></div>
      </aside>
    </div>
  {/if}
</div>

{#if openedGame}
  <CopiesDialog
    game={openedGame}
    busy={!!uploading}
    uploadingThis={uploading?.gameId === openedGame.gameId}
    on:close={() => (opened = null)}
    on:putBack={(e) => putBack(openedGame, e.detail)}
    on:upload={() => upload([openedGame])}
    on:deleted={read}
  />
{/if}

<style>
  /* As wide as the window, like the Library; a cap left full-screen windows
     with an empty strip down the right. */
  .cols {
    display: flex;
    gap: 20px;
    align-items: flex-start;
  }
  .main {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 18px;
  }
  .side {
    width: 320px;
    flex: none;
    display: flex;
    flex-direction: column;
    gap: 16px;
  }
  /* One after another: the header, the list, then the cards on the side. */
  .part {
    animation: arrive 0.34s cubic-bezier(0.2, 0.7, 0.2, 1) backwards;
    animation-delay: calc(var(--n) * 55ms);
  }
  /* Narrow windows: the side cards go under the list. */
  @media (max-width: 1180px) {
    .cols {
      flex-direction: column;
      align-items: stretch;
    }
    .side {
      width: auto;
      display: grid;
      grid-template-columns: 1fr 1fr;
      align-items: start;
    }
  }
  @media (max-width: 760px) {
    .side {
      grid-template-columns: 1fr;
    }
  }
  .shelf {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .shelf-head {
    display: flex;
    align-items: center;
    gap: 12px;
    flex-wrap: wrap;
    min-height: 36px;
  }
  h2 {
    font-size: var(--text-lg);
    font-weight: 600;
  }
  .quiet-box {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 22px;
    border-radius: var(--radius-lg);
    border: 1.5px dashed var(--border-strong);
    color: var(--text-dim);
  }
</style>
