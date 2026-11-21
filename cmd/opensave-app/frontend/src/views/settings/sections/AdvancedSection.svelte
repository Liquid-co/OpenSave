<script>
  // Advanced: the daemon's port, rules that turn one device's paths into
  // this one's, where the scan looks and does not, and untracking every
  // game to start over.
  import SettingGroup from '../../../components/ui/SettingGroup.svelte';
  import SettingRow from '../../../components/ui/SettingRow.svelte';
  import FolderBrowser from '../../../components/FolderBrowser.svelte';
  import ArrowRight from 'lucide-svelte/icons/arrow-right';
  import Plus from 'lucide-svelte/icons/plus';
  import X from 'lucide-svelte/icons/x';
  import { api, native } from '../../../lib/api.js';
  import { toast, askConfirm, gameList } from '../../../lib/stores.js';
  import { collapse, rise } from '../../../lib/motion.js';
  import { t } from '../../../lib/i18n.js';

  export let draft;
  export let unsaved = [];
  export let save = () => {};

  // ── Path rules ──
  function addRule() {
    draft.pathTranslations = [...(draft.pathTranslations ?? []), { fromPattern: '', toPattern: '' }];
  }
  function removeRule(i) {
    draft.pathTranslations = draft.pathTranslations.filter((_, idx) => idx !== i);
    save();
  }

  // ── Folders the scan looks in, and skips ──
  let browsing = null; // 'customScanPaths' | 'excludePaths' | null
  async function addFolder(key, title) {
    if (!native.isWails()) {
      browsing = browsing === key ? null : key;
      return;
    }
    const dir = await native.selectDirectory(title);
    if (dir) add(key, dir);
  }
  function add(key, dir) {
    if (!(draft[key] ?? []).includes(dir)) draft[key] = [...(draft[key] ?? []), dir];
    browsing = null;
    save();
  }
  function removeFolder(key, i) {
    draft[key] = draft[key].filter((_, idx) => idx !== i);
    save();
  }

  // ── Starting over ──
  // Untracks every game so they can be added again from the right places.
  // The snapshots on disk stay.
  let resetting = false;
  async function resetTracking() {
    const n = $gameList.length;
    if (n === 0) return;
    const ok = await askConfirm($t('settings.reset.confirmBody', { n }), {
      title: $t('settings.reset.confirmTitle'),
      confirmText: $t('settings.reset.confirmAction', { n }),
      danger: true
    });
    if (!ok) return;
    resetting = true;
    try {
      const res = await api.post('/api/games/untrack-bulk', { all: true });
      toast($t('settings.reset.doneToast', { n: res.untracked }), 'success');
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      resetting = false;
    }
  }
</script>

<SettingGroup title={$t('settings.advanced.network')}>
  <SettingRow id="port" label={$t('settings.advanced.daemonPort')} hint={$t('settings.advanced.daemonPortHint')} control="s-port">
    <input id="s-port" type="number" class="num" class:invalid={unsaved.includes('port')} bind:value={draft.port} />
  </SettingRow>
</SettingGroup>

<SettingGroup>
  <SettingRow id="pathTranslation" label={$t('settings.advanced.pathTranslation')} hint={$t('settings.advanced.pathTranslationHint')} under>
    {#each draft.pathTranslations ?? [] as rule, i}
      <div class="rule" in:rise={{ y: 4 }}>
        <input placeholder={$t('settings.advanced.fromPattern')} bind:value={rule.fromPattern} spellcheck="false" />
        <span class="arrow"><ArrowRight size={15} /></span>
        <input placeholder={$t('settings.advanced.toPattern')} bind:value={rule.toPattern} spellcheck="false" />
        <button class="icon-btn" title={$t('settings.remove')} aria-label={$t('settings.remove')} on:click={() => removeRule(i)}><X size={15} /></button>
      </div>
    {/each}
    <button class="btn small add" on:click={addRule}><Plus size={14} />{$t('settings.advanced.addRule')}</button>
  </SettingRow>
</SettingGroup>

<SettingGroup title={$t('settings.storage.scanner')}>
  {#each [['customScanPaths', 'scanPaths', 'settings.storage.scanPaths', 'settings.storage.scanPathsHint', 'settings.storage.addFolder', 'settings.storage.scanAddDialog'], ['excludePaths', 'excludePaths', 'settings.storage.excludePaths', 'settings.storage.excludePathsHint', 'settings.storage.excludeFolder', 'settings.storage.excludeAddDialog']] as [key, id, label, hint, addLabel, dialog]}
    <SettingRow {id} label={$t(label)} hint={$t(hint)} under>
      {#each draft[key] ?? [] as p, i (p)}
        <div class="folder" in:rise={{ y: 4 }}>
          <code title={p}>{p}</code>
          <button class="icon-btn" title={$t('settings.remove')} aria-label={$t('settings.remove')} on:click={() => removeFolder(key, i)}><X size={15} /></button>
        </div>
      {/each}
      <button class="btn small add" class:on={browsing === key} on:click={() => addFolder(key, $t(dialog))}><Plus size={14} />{$t(addLabel)}</button>
      {#if browsing === key}
        <div transition:collapse><FolderBrowser on:pick={(e) => add(key, e.detail)} /></div>
      {/if}
    </SettingRow>
  {/each}
</SettingGroup>

<SettingGroup>
  <SettingRow id="reset" label={$t('settings.reset.title')} hint={$t('settings.reset.hint')} danger>
    <button class="btn small danger" on:click={resetTracking} disabled={resetting || $gameList.length === 0}>
      {#if resetting}{$t('settings.reset.inProgress')}{:else if $gameList.length === 0}{$t('settings.reset.none')}{:else}{$t('settings.reset.button', { n: $gameList.length })}{/if}
    </button>
  </SettingRow>
</SettingGroup>

<style>
  .num {
    width: 110px;
  }
  .invalid {
    border-color: var(--danger);
  }
  .rule,
  .folder {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .rule input {
    flex: 1;
    min-width: 0;
  }
  .arrow {
    display: grid;
    color: var(--text-faint);
  }
  .folder code {
    flex: 1;
    min-width: 0;
    padding: 7px 10px;
    border-radius: 8px;
    background: var(--bg);
    border: 1px solid var(--border);
    font-size: var(--text-sm);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    direction: rtl;
    text-align: left;
  }
  .add {
    align-self: flex-start;
  }
  .btn.on:not(.primary) {
    border-color: var(--accent);
    color: var(--accent-text);
  }
  .icon-btn {
    flex: none;
    width: 30px;
    height: 30px;
    display: grid;
    place-items: center;
    border: 0;
    border-radius: 8px;
    background: transparent;
    color: var(--text-faint);
    cursor: pointer;
  }
  .icon-btn:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
</style>
