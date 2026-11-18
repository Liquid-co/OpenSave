<script>
  // Per-game settings, edited as a form and saved together with one button.
  import { askConfirm } from '../../lib/stores.js';
  import { api, native, coverURL } from '../../lib/api.js';
  import { savePathChange } from '../../lib/savepath.js';
  import { t } from '../../lib/i18n.js';
  import AppIdField from './AppIdField.svelte';
  import SyncIgnoreEditor from './SyncIgnoreEditor.svelte';
  import SaveLocations from './SaveLocations.svelte';
  import MoreInfo from '../../components/ui/MoreInfo.svelte';
  import Gamepad2 from 'lucide-svelte/icons/gamepad-2';

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

    await run($t('game.config.saved'), () =>
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
  }

  async function browseSavePath() {
    const dir = await native.selectDirectory($t('game.config.saveFolderDialog', { name: game.name }));
    if (dir) cfg.savePath = dir;
  }

  async function browseExe() {
    const file = await native.selectFile($t('game.config.selectExe'));
    if (file) cfg.exePath = file;
  }
</script>

<div class="card config">
  <div class="cover">
    {#if cover}
      <img src={cover} alt="" on:error={(e) => (e.currentTarget.style.display = 'none')} />
    {:else}
      <div class="cover-fallback"><Gamepad2 size={30} strokeWidth={1.5} /></div>
    {/if}
  </div>
  <div class="fields">
    <h3>{$t('game.config.title')}</h3>
    <div class="field">
      <label for="c-savepath">{$t('game.config.saveFolder')}</label>
      <div class="path-row">
        <input id="c-savepath" placeholder={$t('game.config.saveFolderPlaceholder')} bind:value={cfg.savePath} />
        <button class="btn" on:click={browseSavePath}>{$t('game.header.browse')}</button>
      </div>
      <span class="hint">
        {$t('game.config.saveFolderHint')}
        <MoreInfo>
          {$t('game.config.saveFolderMore1')}<em>{$t('game.config.saveFolderMoreEm')}</em>{$t('game.config.saveFolderMore2')}
        </MoreInfo>
      </span>
      {#if savePathChange(game.savePath, cfg.savePath)}
        <span class="hint hint-warn">
          {$t('game.config.saveFolderNotSaved')}
        </span>
      {/if}
    </div>
    <AppIdField bind:value={cfg.appId} error={appIdError} />
    <div class="field">
      <label for="c-exe">{$t('game.config.exePath')}</label>
      <div class="path-row">
        <input id="c-exe" placeholder={$t('game.config.exePathPlaceholder')} bind:value={cfg.exePath} />
        <button class="btn" on:click={browseExe}>{$t('game.header.browse')}</button>
      </div>
      <span class="hint">
        {$t('game.config.exePathHint')}
      </span>
    </div>
    <div class="field">
      <label for="c-cover">{$t('game.config.coverUrl')}</label>
      <input id="c-cover" placeholder={$t('game.config.coverUrlPlaceholder')} bind:value={cfg.coverUrl} />
      <span class="hint">{$t('game.config.coverUrlHint')}</span>
    </div>
    <label class="check">
      <input type="checkbox" bind:checked={cfg.autoSync} />
      {$t('game.config.autoSync')}
    </label>
    <div class="field" style="margin-top: 12px;">
      <label for="c-max">{$t('game.config.maxSnapshots')}</label>
      <input id="c-max" type="number" min="0" bind:value={cfg.maxSnapshots} />
      <span class="hint">
        {$t('game.config.maxHint1')}<em>{$t('game.config.maxHintEm')}</em>{$t('game.config.maxHint2')}
      </span>
    </div>
    <div class="field">
      <label for="c-max-manual">{$t('game.config.maxManualSnapshots')}</label>
      <input id="c-max-manual" type="number" min="0" bind:value={cfg.maxManualSnapshots} />
      <span class="hint">
        {$t('game.config.manualHint1')}<strong>{$t('game.config.manualHintStrong')}</strong>{$t('game.config.manualHint2')}
      </span>
    </div>
    {#if deviceSettings.length > 0}
      <div class="field device-block">
        <label for="c-devsettings">{$t('game.config.deviceSettings')}</label>
        <ul class="device-settings">
          {#each deviceSettings as p}
            <li><code>{p.replace(/^\//, '')}</code></li>
          {/each}
        </ul>
      </div>
      <label class="check">
        <input id="c-devsettings" type="checkbox" bind:checked={cfg.syncDeviceSettings} />
        {$t('game.config.syncDeviceSettings')}
      </label>
      <span class="hint device-hint">{$t('game.config.deviceSettingsHint')}</span>
    {/if}
    <SyncIgnoreEditor {game} bind:value={cfg.syncIgnore} severalFolders={locations.length > 0} />
    <SaveLocations {game} {runner} bind:locations />
    <div class="save">
      <button class="btn primary" disabled={$busy} on:click={save}>{$t('game.config.saveBtn')}</button>
    </div>
  </div>
</div>

<style>
  .config {
    display: flex;
    gap: 20px;
    align-items: flex-start;
  }
  .cover {
    flex-shrink: 0;
    width: 160px;
    aspect-ratio: 460 / 215;
    border-radius: var(--radius);
    overflow: hidden;
    border: 1px solid var(--border);
    background: var(--bg);
  }
  .cover img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .cover-fallback {
    width: 100%;
    height: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--text-faint);
  }
  .fields {
    flex: 1;
    min-width: 0;
  }
  .fields h3 {
    margin-bottom: 14px;
  }
  .fields .field {
    margin-bottom: 14px;
  }
  .device-settings {
    margin: 4px 0 6px;
    padding-left: 18px;
    color: var(--text-dim, inherit);
  }
  .device-settings code {
    font-size: 0.9em;
  }
  .device-block {
    margin-bottom: 4px !important;
  }
  .device-hint {
    display: block;
    margin-bottom: 14px;
  }
  .hint-warn {
    color: var(--warn);
  }
  .save {
    display: flex;
    justify-content: flex-end;
    margin-top: 8px;
  }
  @media (max-width: 720px) {
    .config {
      flex-direction: column;
    }
    .cover {
      width: 100%;
    }
  }
</style>
