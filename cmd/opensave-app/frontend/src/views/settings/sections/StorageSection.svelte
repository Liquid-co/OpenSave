<script>
  // Snapshots and storage: the space they take and whether they can be put
  // back, where they are kept, how long and how many.
  import SettingGroup from '../../../components/ui/SettingGroup.svelte';
  import SettingRow from '../../../components/ui/SettingRow.svelte';
  import Switch from '../../../components/ui/Switch.svelte';
  import FolderBrowser from '../../../components/FolderBrowser.svelte';
  import StorageUsage from '../StorageUsage.svelte';
  import SnapshotChecks from '../SnapshotChecks.svelte';
  import FolderOpen from 'lucide-svelte/icons/folder-open';
  import BrushCleaning from 'lucide-svelte/icons/brush-cleaning';
  import { api, native } from '../../../lib/api.js';
  import { toast } from '../../../lib/stores.js';
  import { collapse } from '../../../lib/motion.js';
  import { t, segments } from '../../../lib/i18n.js';

  export let draft;
  export let unsaved = [];
  export let save = () => {};
  /** Saves now and says how that went (lib/autosave.js). */
  export let flush = async () => ({});

  // Where a folder is chosen: the system's picker in the app, the folder
  // navigator in the browser build.
  let browsing = null; // 'backupsDir' | 'syncBackupsDir' | null
  async function pick(key, title) {
    if (!native.isWails()) {
      browsing = browsing === key ? null : key;
      return;
    }
    const dir = await native.selectDirectory(title);
    if (dir) {
      draft[key] = dir;
      save();
    }
  }
  function picked(key, dir) {
    draft[key] = dir;
    browsing = null;
    save();
  }

  let pruning = false;
  async function applyLimits() {
    pruning = true;
    try {
      // The limit is saved first, so the clean-up uses it.
      const state = await flush();
      if (state?.error) throw new Error($t('settings.cleanUp.saveFailed', { error: state.error }));
      const res = await api.post('/api/snapshots/prune', { applyDefaultToAll: true });
      const mb = (res.freedBytes / 1048576).toFixed(1);
      toast(res.removed > 0 ? $t('settings.cleanUp.removed', { n: res.removed, mb }) : $t('settings.cleanUp.none'), 'success');
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      pruning = false;
    }
  }
</script>

<SettingGroup>
  <SettingRow id="spaceUsed" label={$t('settings.storage.spaceUsed')} under>
    <StorageUsage />
  </SettingRow>
  <SettingRow id="canRestore" label={$t('settings.storage.canRestore')} under>
    <SnapshotChecks />
  </SettingRow>
</SettingGroup>

<SettingGroup title={$t('settings.storage.snapshotStorage')}>
  <SettingRow id="snapshotsFolder" label={$t('settings.storage.snapshotsFolder')} hint={$t('settings.storage.snapshotsFolderHint')} control="s-backups" under>
    <div class="path-row">
      <input id="s-backups" class:invalid={unsaved.includes('backupsDir')} bind:value={draft.backupsDir} spellcheck="false" />
      <button class="btn small" class:on={browsing === 'backupsDir'} on:click={() => pick('backupsDir', $t('settings.storage.pickDialog'))}><FolderOpen size={14} />{$t('settings.storage.browse')}</button>
    </div>
    {#if browsing === 'backupsDir'}
      <div transition:collapse><FolderBrowser start={draft.backupsDir} on:pick={(e) => picked('backupsDir', e.detail)} /></div>
    {/if}
  </SettingRow>
  <SettingRow id="safetyFolder" label={$t('settings.storage.safetyFolder')} hint={$t('settings.storage.safetyFolderHint')} control="s-sync-backups" under>
    <div class="path-row">
      <input id="s-sync-backups" class:invalid={unsaved.includes('syncBackupsDir')} bind:value={draft.syncBackupsDir} placeholder={$t('settings.storage.safetyFolderPlaceholder')} spellcheck="false" />
      <button class="btn small" class:on={browsing === 'syncBackupsDir'} on:click={() => pick('syncBackupsDir', $t('settings.storage.pickSyncDialog'))}><FolderOpen size={14} />{$t('settings.storage.browse')}</button>
    </div>
    {#if browsing === 'syncBackupsDir'}
      <div transition:collapse><FolderBrowser start={draft.syncBackupsDir} on:pick={(e) => picked('syncBackupsDir', e.detail)} /></div>
    {/if}
  </SettingRow>
</SettingGroup>

<SettingGroup title={$t('settings.storage.retention')}>
  <SettingRow id="autoDelete" label={$t('settings.storage.autoDelete')} hint={$t('settings.storage.autoDeleteHint')}>
    <Switch
      checked={draft.autoDeleteBackups}
      label={$t('settings.storage.autoDelete')}
      on:change={(e) => {
        draft.autoDeleteBackups = e.detail;
        save();
      }}
    />
  </SettingRow>
  {#if draft.autoDeleteBackups}
    <div transition:collapse>
      <SettingRow label={$t('settings.storage.deleteOlderThan')} control="s-days">
        <select id="s-days" bind:value={draft.autoDeleteDays}>
          {#each [7, 14, 30, 60, 90, 180] as n}<option value={n}>{$t('settings.storage.days', { n })}</option>{/each}
        </select>
      </SettingRow>
    </div>
  {/if}
</SettingGroup>

<SettingGroup title={$t('settings.storage.snapshotHistory')}>
  <SettingRow id="maxAuto" label={$t('settings.storage.maxAuto')} hint={$t('settings.storage.maxAutoHint')} control="s-max-snaps">
    <input id="s-max-snaps" type="number" min="0" class="num" bind:value={draft.defaultMaxSnapshots} />
  </SettingRow>
  <SettingRow id="maxManual" label={$t('settings.storage.maxManual')} control="s-max-manual-snaps">
    <svelte:fragment slot="hint">
      {#each segments($t('settings.storage.maxManualHint'), { forever: $t('settings.storage.keepForever') }, ['forever']) as part}{#if part.bold}<strong>{part.text}</strong>{:else}{part.text}{/if}{/each}
    </svelte:fragment>
    <input id="s-max-manual-snaps" type="number" min="0" class="num" bind:value={draft.defaultMaxManualSnapshots} />
  </SettingRow>
  <SettingRow id="applyLimits" label={$t('settings.storage.applyLimits')} hint={$t('settings.storage.applyLimitsHint')}>
    <button class="btn small" disabled={pruning} on:click={applyLimits}>
      <BrushCleaning size={14} />{pruning ? $t('settings.storage.applying') : $t('settings.storage.applyLimits')}
    </button>
  </SettingRow>
</SettingGroup>

<style>
  .path-row {
    display: flex;
    gap: 8px;
  }
  .path-row input {
    flex: 1;
    min-width: 0;
  }
  .num {
    width: 110px;
  }
  .invalid {
    border-color: var(--danger);
  }
  .btn.on:not(.primary) {
    border-color: var(--accent);
    color: var(--accent-text);
  }
</style>
