<script>
  // Per-game settings, edited as a form and saved together with one button.
  import { askConfirm } from '../../lib/stores.js';
  import { api, native, coverURL } from '../../lib/api.js';
  import { savePathChange } from '../../lib/savepath.js';
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
    coverUrl: game.coverUrl ?? '',
    autoSync: game.autoSync ?? true,
    maxSnapshots: game.maxSnapshots ?? 5,
    maxManualSnapshots: game.maxManualSnapshots ?? 0,
    syncIgnore: game.syncIgnore ?? ''
  };
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
      appIdError = 'A Steam App ID is digits only — the number in the store URL.';
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
        `Track “${game.name}” at ${savePath} from now on?\n\n` +
          `Files in the old folder are left exactly where they are, but they stop syncing ` +
          `and stop being snapshotted. Existing snapshots are kept and still restore.`,
        { title: 'Move tracking to a new folder?', confirmText: 'Move tracking' }
      );
      if (!ok) return;
    }

    await run('Configuration saved', () =>
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
        syncIgnore: cfg.syncIgnore ?? ''
      })
    );
  }

  async function browseSavePath() {
    const dir = await native.selectDirectory(`Save folder for “${game.name}”`);
    if (dir) cfg.savePath = dir;
  }

  async function browseExe() {
    const file = await native.selectFile('Select the game executable');
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
    <h3>Launch &amp; sync configuration</h3>
    <div class="field">
      <label for="c-savepath">Save folder</label>
      <div class="path-row">
        <input id="c-savepath" placeholder="Folder holding this game's saves" bind:value={cfg.savePath} />
        <button class="btn" on:click={browseSavePath}>Browse</button>
      </div>
      <span class="hint">
        The folder OpenSave treats as this game's save.
        <MoreInfo>
          Files sync by their position <em>inside</em> this folder, so each device can point at
          a different path and still end up with the same files — useful when a game keeps saves
          in a per-account folder named after your Steam or Epic ID, which differs on every
          machine. Point each device at its own such folder and saves land where that copy of
          the game looks for them.
        </MoreInfo>
      </span>
      {#if savePathChange(game.savePath, cfg.savePath)}
        <span class="hint hint-warn">
          Not saved yet. The old folder keeps its files but stops syncing and being
          snapshotted.
        </span>
      {/if}
    </div>
    <AppIdField bind:value={cfg.appId} error={appIdError} />
    <div class="field">
      <label for="c-exe">Executable path</label>
      <div class="path-row">
        <input id="c-exe" placeholder="Browse to the game .exe" bind:value={cfg.exePath} />
        <button class="btn" on:click={browseExe}>Browse</button>
      </div>
      <span class="hint">
        Launch starts this, even for a Steam game. Leave it empty to launch through Steam.
      </span>
    </div>
    <div class="field">
      <label for="c-cover">Custom cover image URL</label>
      <input id="c-cover" placeholder="https://…  (great for emulator games)" bind:value={cfg.coverUrl} />
      <span class="hint">Leave blank to auto-use Steam art from the App ID. Paste any image URL for emulators.</span>
    </div>
    <label class="check">
      <input type="checkbox" bind:checked={cfg.autoSync} />
      Auto-sync saves when changes are detected
    </label>
    <div class="field" style="margin-top: 12px;">
      <label for="c-max">Automatic snapshot limit</label>
      <input id="c-max" type="number" min="0" bind:value={cfg.maxSnapshots} />
      <span class="hint">
        Max <em>automatic</em> snapshots kept per branch (0 = unlimited). Oldest are pruned first.
      </span>
    </div>
    <div class="field">
      <label for="c-max-manual">Manual snapshot limit</label>
      <input id="c-max-manual" type="number" min="0" bind:value={cfg.maxManualSnapshots} />
      <span class="hint">
        Snapshots you took yourself get their own budget, so a game that auto-saves often
        can't push them out. <strong>0 = keep forever</strong> (the default).
      </span>
    </div>
    <SyncIgnoreEditor {game} bind:value={cfg.syncIgnore} severalFolders={locations.length > 0} />
    <SaveLocations {game} {runner} bind:locations />
    <div class="save">
      <button class="btn primary" disabled={$busy} on:click={save}>Save configuration</button>
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
