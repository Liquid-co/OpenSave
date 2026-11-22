<script context="module">
  // A game's settings changed here and not saved, kept when its page is left
  // (gameId -> the form as it was) and given back when its Settings tab is
  // opened again. Leaving used to throw them away without a word.
  const drafts = new Map();
</script>

<script>
  // Settings: how OpenSave treats this game, in groups — where its save is,
  // how it launches and looks, its snapshots and syncing — and, at the end,
  // stopping tracking it. Changes wait for Save, in a bar that appears once
  // there is something to save.
  import { onDestroy } from 'svelte';
  import { get } from 'svelte/store';
  import { askConfirm, toast, navigate, games } from '../../lib/stores.js';
  import FolderBrowser from '../../components/FolderBrowser.svelte';
  import Switch from '../../components/ui/Switch.svelte';
  import { collapse } from '../../lib/motion.js';
  import FolderOpen from 'lucide-svelte/icons/folder-open';
  import Save from 'lucide-svelte/icons/save';
  import { api, native, coverURL } from '../../lib/api.js';
  import { savePathChange } from '../../lib/savepath.js';
  import { t } from '../../lib/i18n.js';
  import AppIdField from './AppIdField.svelte';
  import SyncIgnoreEditor from './SyncIgnoreEditor.svelte';
  import SaveLocations from './SaveLocations.svelte';
  import MoreInfo from '../../components/ui/MoreInfo.svelte';

  export let game;
  export let runner;
  const { busy, run } = runner;

  // Taken from the game once, when the tab opens: the game updates often
  // (every sync), and re-reading it would throw away what is being typed.
  let cfg = {
    appId: game.appId ?? '',
    savePath: game.savePath ?? '',
    exePath: game.exePath ?? '',
    // The art Steam has for the App ID is not a custom cover: shown here it
    // read as one, beside a hint saying to leave the field blank for it.
    coverUrl: game.coverUrl && !game.coverUrl.includes('steamstatic.com/steam/apps/') ? game.coverUrl : '',
    autoSync: game.autoSync ?? true,
    maxSnapshots: game.maxSnapshots ?? 5,
    maxManualSnapshots: game.maxManualSnapshots ?? 0,
    syncIgnore: game.syncIgnore ?? '',
    syncDeviceSettings: game.syncDeviceSettings ?? false
  };
  // The files the game database names as this game's settings rather than its
  // save (graphics, resolution): left out of syncing unless asked for.
  $: deviceSettings = game.deviceSettings ?? [];
  let locations = [];

  // What was last saved, to tell whether anything waits to be.
  let saved = JSON.stringify(cfg);
  $: dirty = JSON.stringify(cfg) !== saved;
  function discard() {
    cfg = JSON.parse(saved);
    appIdError = '';
  }
  // Changes left unsaved last time this game's page was open: back, still
  // unsaved, with the bar that saves them.
  if (drafts.has(game.id)) {
    cfg = drafts.get(game.id);
    drafts.delete(game.id);
  }
  // Leaving with changes not saved: they are kept for coming back to, and the
  // toast says so, with a way straight back.
  onDestroy(() => {
    if (!dirty || !get(games)[game.id]) return;
    const id = game.id;
    drafts.set(id, cfg);
    toast($t('game.settings.keptUnsaved', { name: game.name }), 'info', {
      ttl: 10000,
      action: { label: $t('game.settings.goBack'), run: () => navigate('game', { gameId: id, tab: 'settings' }) }
    });
  });
  // Undone from the toast rather than confirmed first; see lib/undo.js.

  // No system picker in the browser build: the folder navigator instead.
  let browsingSave = false;

  // Cover preview: a custom URL wins, else the proxied Steam art for the App
  // ID being edited.
  $: cover =
    cfg.coverUrl && !cfg.coverUrl.includes('steamstatic.com') ? cfg.coverUrl : coverURL(cfg.appId, false, '');

  // A Steam App ID is digits. Saving something else stores a value that
  // matches nothing, launches nothing and fetches no art, with no complaint
  // at any point — which reads as the features being broken rather than the
  // field being wrong. Catch it at the edit instead.
  let appIdError = '';

  async function save() {
    const appId = (cfg.appId ?? '').trim();
    if (appId !== '' && !/^\d+$/.test(appId)) {
      appIdError = $t('game.config.appIdError');
      return;
    }
    appIdError = '';

    // Moving tracking to a different folder is the one edit on this form that
    // changes WHICH files are the save, rather than a detail about them. The
    // old folder is left untouched, but nothing there syncs or is snapshotted
    // any more, so it is worth one confirmation.
    const savePath = savePathChange(game.savePath, cfg.savePath);
    if (savePath) {
      const ok = await askConfirm(
        $t('game.config.moveTrackConfirm', { name: game.name, path: savePath }),
        { title: $t('game.config.moveTrackTitle'), confirmText: $t('game.config.moveTrackConfirmBtn') }
      );
      if (!ok) return;
    }

    const ok = await run($t('game.config.saved'), () =>
      api.patch(`/api/games/${game.id}`, {
        appId,
        // Omitted unless it actually changed: the PATCH decodes into the
        // stored game, so leaving it out keeps the current folder. Sending an
        // empty string instead would be a real edit, and validation would
        // reject it — blocking every other change on this form.
        ...(savePath ? { savePath } : {}),
        exePath: cfg.exePath,
        coverUrl: cfg.coverUrl,
        autoSync: cfg.autoSync,
        maxSnapshots: Number(cfg.maxSnapshots),
        maxManualSnapshots: Number(cfg.maxManualSnapshots),
        syncIgnore: cfg.syncIgnore ?? '',
        syncDeviceSettings: !!cfg.syncDeviceSettings
      })
    );
    if (ok) saved = JSON.stringify(cfg);
  }

  async function browseSavePath() {
    if (!native.isWails()) {
      browsingSave = !browsingSave;
      return;
    }
    const dir = await native.selectDirectory($t('game.config.saveFolderDialog', { name: game.name }));
    if (dir) cfg.savePath = dir;
  }

  // Choosing the program opens where it is: the folder of the one set, or
  // where the game is installed when the daemon knows (Steam's folder for
  // it), or otherwise where programs are installed. The desktop app opens the
  // system's own picker, offering what this system counts as a program; the
  // browser build has the folder navigator do the same.
  let browsingExe = false;
  let exeFrom = '';
  const folderOf = (p) => {
    const dir = String(p ?? '').trim().replace(/[\\/][^\\/]*$/, '');
    return /^[A-Za-z]:$/.test(dir) ? dir + '\\' : dir;
  };
  async function exeStart() {
    if (cfg.exePath?.trim()) return folderOf(cfg.exePath);
    try {
      return (await api.get(`/api/games/${game.id}/install-folder`)).dir ?? '';
    } catch {
      return '';
    }
  }
  async function browseExe() {
    if (browsingExe) {
      browsingExe = false;
      return;
    }
    const from = await exeStart();
    if (!native.isWails()) {
      exeFrom = from;
      browsingExe = true;
      return;
    }
    const file = await native.selectExecutable($t('game.config.selectExe'), from);
    if (file) cfg.exePath = file;
  }
</script>

<div class="sections">
  <section class="card sec" style="--i: 0" aria-labelledby="set-save">
    <h3 id="set-save">{$t('game.settings.saveTitle')}</h3>
    <div class="field">
      <label for="c-savepath">{$t('game.config.saveFolder')}</label>
      <div class="path-row">
        <input id="c-savepath" placeholder={$t('game.config.saveFolderPlaceholder')} bind:value={cfg.savePath} />
        <button class="btn" class:on={browsingSave} on:click={browseSavePath}><FolderOpen size={15} />{$t('game.header.browse')}</button>
      </div>
      {#if browsingSave}
        <div transition:collapse>
          <FolderBrowser
            start={cfg.savePath}
            on:pick={(e) => {
              cfg.savePath = e.detail;
              browsingSave = false;
            }}
          />
        </div>
      {/if}
      <span class="hint">
        {$t('game.config.saveFolderHint')}
        <MoreInfo>
          {$t('game.config.saveFolderMore1')}<em>{$t('game.config.saveFolderMoreEm')}</em>{$t('game.config.saveFolderMore2')}
        </MoreInfo>
      </span>
      {#if savePathChange(game.savePath, cfg.savePath)}
        <span class="hint hint-warn">{$t('game.config.saveFolderNotSaved')}</span>
      {/if}
    </div>
    <SaveLocations {game} {runner} bind:locations />
    {#if deviceSettings.length > 0}
      <div class="field">
        <span class="label">{$t('game.config.deviceSettings')}</span>
        <ul class="device-settings">
          {#each deviceSettings as p}
            <li><code>{p.replace(/^\//, '')}</code></li>
          {/each}
        </ul>
        <div class="switch-row">
          <span class="switch-text">{$t('game.config.syncDeviceSettings')}<span class="hint">{$t('game.config.deviceSettingsHint')}</span></span>
          <Switch checked={cfg.syncDeviceSettings} label={$t('game.config.syncDeviceSettings')} on:change={(e) => (cfg.syncDeviceSettings = e.detail)} />
        </div>
      </div>
    {/if}
    <SyncIgnoreEditor {game} bind:value={cfg.syncIgnore} severalFolders={locations.length > 0} />
  </section>

  <section class="card sec" style="--i: 1" aria-labelledby="set-sync">
    <h3 id="set-sync">{$t('game.settings.syncTitle')}</h3>
    <div class="switch-row">
      <span class="switch-text">{$t('game.config.autoSync')}<span class="hint">{$t('game.settings.autoSyncHint')}</span></span>
      <Switch checked={cfg.autoSync} label={$t('game.config.autoSync')} on:change={(e) => (cfg.autoSync = e.detail)} />
    </div>
    <div class="two">
      <div class="field">
        <label for="c-max">{$t('game.config.maxSnapshots')}</label>
        <input id="c-max" type="number" min="0" bind:value={cfg.maxSnapshots} />
        <span class="hint">{$t('game.config.maxHint1')}<em>{$t('game.config.maxHintEm')}</em>{$t('game.config.maxHint2')}</span>
      </div>
      <div class="field">
        <label for="c-max-manual">{$t('game.config.maxManualSnapshots')}</label>
        <input id="c-max-manual" type="number" min="0" bind:value={cfg.maxManualSnapshots} />
        <span class="hint">{$t('game.config.manualHint1')}<strong>{$t('game.config.manualHintStrong')}</strong>{$t('game.config.manualHint2')}</span>
      </div>
    </div>
  </section>

  <section class="card sec" style="--i: 2" aria-labelledby="set-launch">
    <h3 id="set-launch">{$t('game.settings.launchTitle')}</h3>
    <div class="field">
      <label for="c-exe">{$t('game.config.exePath')}</label>
      <div class="path-row">
        <input id="c-exe" placeholder={$t('game.config.exePathPlaceholder')} bind:value={cfg.exePath} />
        <button class="btn" class:on={browsingExe} on:click={browseExe}><FolderOpen size={15} />{$t('game.header.browse')}</button>
      </div>
      {#if browsingExe}
        <div transition:collapse>
          <FolderBrowser
            start={exeFrom}
            files="programs"
            on:pick={(e) => {
              cfg.exePath = e.detail;
              browsingExe = false;
            }}
          />
        </div>
      {/if}
      <span class="hint">{$t('game.config.exePathHint')}</span>
    </div>
    <AppIdField bind:value={cfg.appId} error={appIdError} />
    <div class="field cover-field">
      <label for="c-cover">{$t('game.config.coverUrl')}</label>
      <div class="cover-row">
        {#if cover}<img class="cover-prev" src={cover} alt="" on:error={(e) => (e.currentTarget.style.display = 'none')} />{/if}
        <input id="c-cover" placeholder={$t('game.config.coverUrlPlaceholder')} bind:value={cfg.coverUrl} />
      </div>
      <span class="hint">{$t('game.config.coverUrlHint')}</span>
    </div>
  </section>
</div>

{#if dirty}
  <div class="savebar" transition:collapse>
    <span>{$t('game.settings.unsaved')}</span>
    <button class="btn ghost" on:click={discard}>{$t('game.settings.discard')}</button>
    <button class="btn primary" disabled={$busy} on:click={save}><Save size={15} />{$t('game.config.saveBtn')}</button>
  </div>
{/if}

<style>
  .sections {
    display: flex;
    flex-direction: column;
    gap: 16px;
  }
  .sec {
    display: flex;
    flex-direction: column;
    gap: 14px;
    padding: 18px 20px;
    border-radius: var(--radius-lg);
    animation: arrive 0.34s cubic-bezier(0.2, 0.7, 0.2, 1) backwards;
    animation-delay: calc(var(--i) * 50ms);
  }
  h3 {
    font-size: var(--text-base);
    font-weight: 600;
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .field label,
  .label {
    font-size: var(--text-md);
    font-weight: 600;
    color: var(--text-dim);
  }
  .path-row,
  .cover-row {
    display: flex;
    gap: 8px;
    align-items: center;
  }
  .path-row input,
  .cover-row input {
    flex: 1;
    min-width: 0;
  }
  .btn.on {
    border-color: var(--accent);
    color: var(--accent-text);
  }
  .cover-prev {
    height: 36px;
    aspect-ratio: 460 / 215;
    object-fit: cover;
    border-radius: calc(6px * var(--round, 1));
    border: 1px solid var(--border);
  }
  .two {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 16px;
  }
  @media (max-width: 760px) {
    .two {
      grid-template-columns: 1fr;
    }
  }
  .switch-row {
    display: flex;
    align-items: center;
    gap: 14px;
  }
  .switch-text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    font-weight: 600;
    font-size: var(--text-md);
  }
  .switch-text .hint {
    font-weight: 400;
  }
  .hint {
    font-size: var(--text-sm);
    color: var(--text-faint);
    line-height: 1.45;
  }
  .hint-warn {
    color: var(--warn);
  }
  .device-settings {
    margin: 0;
    padding-left: 18px;
    color: var(--text-dim);
  }
  /* Appears once something is changed, along the foot of the page. */
  .savebar {
    position: sticky;
    bottom: 12px;
    z-index: 5;
    display: flex;
    align-items: center;
    gap: 10px;
    margin-top: 16px;
    padding: 10px 12px 10px 18px;
    border-radius: var(--radius-lg);
    border: 1px solid rgba(var(--game-accent-rgb, var(--accent-rgb)), 0.5);
    background: var(--bg-raised);
    box-shadow: var(--shadow);
  }
  .savebar span {
    flex: 1;
    font-weight: 600;
  }
</style>
