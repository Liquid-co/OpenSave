<script>
  import { slidingIndicator } from '../lib/motion.js';
  import { onMount, onDestroy } from 'svelte';
  import { settings, toast, askConfirm, gameList, navigate } from '../lib/stores.js';
  import { showSetupAgain } from '../lib/setup.js';
  import { api, native } from '../lib/api.js';
  import { t, segments } from '../lib/i18n.js';
  import { adoptOutsideChanges, changedFields, createAutosave } from '../lib/autosave.js';
  import qrcode from 'qrcode-generator';
  import { DISCORD_URL, DONATE_URL } from '../lib/links.js';
  import LibraryViewOptions from './home/LibraryViewOptions.svelte';
  import Skeleton from '../components/ui/Skeleton.svelte';
  import MoreInfo from '../components/ui/MoreInfo.svelte';
  import AppearanceOptions from './settings/AppearanceOptions.svelte';
  import ControllerOptions from './settings/ControllerOptions.svelte';
  import NotificationOptions from './settings/NotificationOptions.svelte';
  import StorageUsage from './settings/StorageUsage.svelte';
  import SnapshotChecks from './settings/SnapshotChecks.svelte';
  import FileCheck from 'lucide-svelte/icons/file-check';
  import PieChart from 'lucide-svelte/icons/chart-pie';
  import Bell from 'lucide-svelte/icons/bell';
  import Palette from 'lucide-svelte/icons/palette';
  import Monitor from 'lucide-svelte/icons/monitor';
  import LayoutGrid from 'lucide-svelte/icons/layout-grid';
  import Rocket from 'lucide-svelte/icons/rocket';
  import Download from 'lucide-svelte/icons/download';
  import RefreshCw from 'lucide-svelte/icons/refresh-cw';
  import Globe from 'lucide-svelte/icons/globe';
  import RadioTower from 'lucide-svelte/icons/radio-tower';
  import Cloud from 'lucide-svelte/icons/cloud';
  import HardDrive from 'lucide-svelte/icons/hard-drive';
  import Timer from 'lucide-svelte/icons/timer';
  import History from 'lucide-svelte/icons/history';
  import BrushCleaning from 'lucide-svelte/icons/brush-cleaning';
  import ScanSearch from 'lucide-svelte/icons/scan-search';
  import RotateCcw from 'lucide-svelte/icons/rotate-ccw';
  import Network from 'lucide-svelte/icons/network';
  import ArrowLeftRight from 'lucide-svelte/icons/arrow-left-right';
  import Heart from 'lucide-svelte/icons/heart';
  import ArrowUpRight from 'lucide-svelte/icons/arrow-up-right';
  import X from 'lucide-svelte/icons/x';
  import Plus from 'lucide-svelte/icons/plus';
  import Wrench from 'lucide-svelte/icons/wrench';
  import Gamepad2 from 'lucide-svelte/icons/gamepad-2';
  import ArrowRight from 'lucide-svelte/icons/arrow-right';
  import Check from 'lucide-svelte/icons/check';
  import LoaderCircle from 'lucide-svelte/icons/loader-circle';
  import TriangleAlert from 'lucide-svelte/icons/triangle-alert';

  // QR of the same URL, generated locally so paying from a phone (where
  // Apple/Google Pay is a single tap) needs no typing. Built once — the URL
  // is constant. Error-correction level M tolerates a bit of screen glare.
  const qrSvg = (() => {
    const qr = qrcode(0, 'M');
    qr.addData(DONATE_URL);
    qr.make();
    return qr.createSvgTag({ cellSize: 4, margin: 0, scalable: true });
  })();

  let donateOpened = false;
  function openDonatePage() {
    native.openExternal(DONATE_URL);
    donateOpened = true;
  }

  /** From navigate('settings', {tab}). */
  export let params = {};
  const TABS = ['general', 'sync', 'storage', 'advanced', 'support'];
  let tab = TABS.includes(params?.tab) ? params.tab : 'general';
  let pruning = false;

  // The running build, so the updates toggle can explain what it means for
  // THIS install: someone already on a beta is offered betas regardless, and
  // saying so is the difference between the checkbox looking broken and
  // looking deliberate.
  let appVersion = '';
  $: onPreRelease = appVersion.includes('-');
  onMount(async () => {
    try {
      appVersion = (await native.appInfo())?.version ?? '';
    } catch {
      appVersion = '';
    }
  });

  // Every change saves itself (see lib/autosave.js): a toggle or a choice at
  // once, typed text when you leave the box or press Enter. `draft` is what
  // the page shows and edits; `saved` is what the daemon last confirmed.
  let draft = null;
  let saved = null;
  let saveState = null; // null | 'saving' | 'saved' | {error}
  // Values the daemon refused, and why, by field: left out of later saves
  // until changed (lib/autosave.js), and shown until then — a later save
  // succeeding does not mean this one did.
  let rejected = {};
  let refusedWhy = {};
  const FIELD_NAMES = {
    relayUrl: 'settings.field.relayUrl',
    relayPort: 'settings.field.relayPort',
    port: 'settings.field.port',
    deviceName: 'settings.field.deviceName',
    backupsDir: 'settings.field.backupsDir',
    syncBackupsDir: 'settings.field.syncBackupsDir',
    pathTranslations: 'settings.field.pathTranslations'
  };
  const fieldName = (key) => (key in FIELD_NAMES ? $t(FIELD_NAMES[key]) : key);
  $: unsaved = draft ? Object.keys(rejected).filter((key) => same(draft[key], rejected[key])) : [];
  const firstSentence = (text) => String(text).split(/(?<=\.)\s/)[0];

  // The settings as this page edits them. The cloud settings are set on the
  // Cloud Backup page and left out: they are not this page's to send.
  function editable(s) {
    const { cloudSync, cloudAutoPull, ...rest } = structuredClone(s);
    // Older daemons predate the separate manual-snapshot budget; 0 is the
    // "keep forever" default, so an omitted value behaves as it should.
    rest.defaultMaxManualSnapshots ??= 0;
    // Older daemons predate the update channel; stable is the default.
    rest.updateChannel ??= 'stable';
    return rest;
  }

  // A path rule with a side still empty is not a rule yet: it stays on the
  // page but is not saved until both sides are filled in.
  const ready = (d) => ({
    ...d,
    pathTranslations: (d.pathTranslations ?? []).filter((r) => r.fromPattern?.trim() && r.toPattern?.trim())
  });

  const same = (a, b) => JSON.stringify(a) === JSON.stringify(b);

  const saver = createAutosave({
    collect: () => changedFields(saved, ready(draft), { rejected }),
    send: (patch) => api.post('/api/settings', patch),
    onSaved(result, patch) {
      const next = editable(result);
      // The daemon's version of what was sent — it may have tidied it — unless
      // the field has been changed again in the meantime.
      for (const key of Object.keys(patch)) {
        if (same(draft[key], patch[key])) draft[key] = structuredClone(next[key]);
        delete rejected[key];
        delete refusedWhy[key];
      }
      rejected = rejected;
      saved = next;
      settings.set(result);
    },
    onFailed(patch, error) {
      rejected = { ...rejected, ...patch };
      for (const key of Object.keys(patch)) refusedWhy[key] = error.message;
    },
    onState: (state) => (saveState = state)
  });
  const saveNow = () => saver.flush();
  onDestroy(saveNow);

  // Arrivals from elsewhere while the page is open — the tray, the terminal,
  // another screen — are taken in, except into a field being edited here.
  function takeSettings(s) {
    if (!s) return;
    if (!draft) {
      saved = editable(s);
      draft = structuredClone(saved);
      return;
    }
    ({ saved, draft } = adoptOutsideChanges(saved, draft, editable(s)));
  }
  $: takeSettings($settings);

  function showTab(next) {
    saveNow();
    tab = next;
  }

  async function cleanUpSnapshots() {
    pruning = true;
    try {
      // Save the limit first so the cleanup uses it, then prune everything.
      await saver.flush();
      if (saveState?.error) throw new Error($t('settings.cleanUp.saveFailed', { error: saveState.error }));
      const res = await api.post('/api/snapshots/prune', { applyDefaultToAll: true });
      const mb = (res.freedBytes / 1048576).toFixed(1);
      toast(
        res.removed > 0
          ? $t('settings.cleanUp.removed', { n: res.removed, mb })
          : $t('settings.cleanUp.none'),
        'success'
      );
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      pruning = false;
    }
  }

  // Path translations editor
  function addRule() {
    draft.pathTranslations = [...(draft.pathTranslations ?? []), { fromPattern: '', toPattern: '' }];
  }
  function removeRule(i) {
    draft.pathTranslations = draft.pathTranslations.filter((_, idx) => idx !== i);
    saveNow();
  }

  // Custom scan paths
  async function addScanPath() {
    const dir = await native.selectDirectory($t('settings.storage.scanAddDialog'));
    if (dir) draft.customScanPaths = [...(draft.customScanPaths ?? []), dir];
    saveNow();
  }
  function removeScanPath(i) {
    draft.customScanPaths = draft.customScanPaths.filter((_, idx) => idx !== i);
    saveNow();
  }

  // Excluded folders — locations the scan should skip entirely.
  async function addExcludePath() {
    const dir = await native.selectDirectory($t('settings.storage.excludeAddDialog'));
    if (dir) draft.excludePaths = [...(draft.excludePaths ?? []), dir];
    saveNow();
  }
  function removeExcludePath(i) {
    draft.excludePaths = draft.excludePaths.filter((_, idx) => idx !== i);
    saveNow();
  }

  // Reset tracking — untrack every game so the user can re-add them from the
  // right locations (e.g. after moving games between launchers or drives).
  // Non-destructive: this only clears the tracked list; snapshot backups on
  // disk are kept.
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

  async function pickBackupsDir() {
    const dir = await native.selectDirectory($t('settings.storage.pickDialog'));
    if (dir) draft.backupsDir = dir;
    saveNow();
  }

  async function pickSyncBackupsDir() {
    const dir = await native.selectDirectory($t('settings.storage.pickSyncDialog'));
    if (dir) draft.syncBackupsDir = dir;
    saveNow();
  }

  // Relay hosting: LAN IPs / public IP to share with friends. Shown only on
  // request — these identify the machine, so they shouldn't appear on screen
  // just because the checkbox was ticked.
  let relayInfo = null;
  let relayInfoShown = false;
  let relayInfoLoading = false;

  async function toggleRelayInfo() {
    if (relayInfoShown) {
      relayInfoShown = false;
      return;
    }
    relayInfoLoading = true;
    try {
      // Re-fetch each time: the public IP changes, and a stale one sent to a
      // friend is worse than none.
      relayInfo = await api.get('/api/relay/ips');
      relayInfoShown = true;
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      relayInfoLoading = false;
    }
  }

  // Turning hosting off should also hide addresses left on screen.
  $: if (draft && !draft.hostRelay && relayInfoShown) relayInfoShown = false;
</script>

<div class="head">
  <h2 class="page-title">{$t('settings.title')}</h2>
  {#if draft}
    <span class="save-state" class:error={saveState?.error || unsaved.length} aria-live="polite">
      {#if saveState === 'saving'}
        <LoaderCircle size={14} class="spin" />{$t('settings.saveState.saving')}
      {:else if unsaved.length}
        <TriangleAlert size={14} />
        <span class="why" title={refusedWhy[unsaved[0]]}>
          {$t('settings.saveState.notSaved', { field: fieldName(unsaved[0]), why: firstSentence(refusedWhy[unsaved[0]]) })}
        </span>
      {:else if saveState?.error}
        <TriangleAlert size={14} /><span class="why" title={saveState.error}>{$t('settings.saveState.saveFailed', { error: firstSentence(saveState.error) })}</span>
      {:else if saveState === 'saved'}
        <Check size={14} />{$t('settings.saveState.saved')}
      {:else}
        {$t('settings.saveState.autosave')}
      {/if}
    </span>
  {/if}
</div>

{#if !draft}
  <Skeleton kind="cards" count={3} />
{:else}
  <div class="pill-tabs" style="margin-bottom: 18px;" use:slidingIndicator={{ inset: 10 }}>
    <button class:active={tab === 'general'} on:click={() => showTab('general')}>{$t('settings.tab.general')}</button>
    <button class:active={tab === 'sync'} on:click={() => showTab('sync')}>{$t('settings.tab.sync')}</button>
    <button class:active={tab === 'storage'} on:click={() => showTab('storage')}>{$t('settings.tab.storage')}</button>
    <button class:active={tab === 'advanced'} on:click={() => showTab('advanced')}>{$t('settings.tab.advanced')}</button>
    <button class="support-tab" class:active={tab === 'support'} on:click={() => showTab('support')}><Heart size={14} />{$t('settings.tab.support')}</button>
    <!-- Not a tab: it leaves the app. Shaped like its neighbour so the pair
         reads as one group, marked with an outward arrow so nobody expects a panel. -->
    <button class="discord-tab" on:click={() => native.openExternal(DISCORD_URL)} title={$t('settings.discord.title')}>
      <span class="discord-glyph" aria-hidden="true">
        <svg viewBox="0 0 24 18" width="17" height="13" fill="currentColor">
          <path d="M20.32 1.53A19.8 19.8 0 0 0 15.43 0c-.21.38-.46.9-.63 1.31a18.3 18.3 0 0 0-5.6 0C9.03.9 8.77.38 8.56 0A19.74 19.74 0 0 0 3.67 1.53C.57 6.19-.27 10.73.15 15.21A19.9 19.9 0 0 0 6.18 18c.49-.66.92-1.37 1.29-2.11-.71-.27-1.39-.6-2.03-.98.17-.13.34-.26.5-.4a14.2 14.2 0 0 0 12.12 0c.16.14.33.27.5.4-.64.38-1.32.71-2.03.98.37.74.8 1.45 1.29 2.11a19.87 19.87 0 0 0 6.03-2.79c.5-5.19-.84-9.69-3.53-13.68ZM8.02 12.46c-1.18 0-2.15-1.08-2.15-2.4s.95-2.4 2.15-2.4c1.2 0 2.17 1.09 2.15 2.4 0 1.32-.95 2.4-2.15 2.4Zm7.96 0c-1.18 0-2.15-1.08-2.15-2.4s.95-2.4 2.15-2.4c1.2 0 2.17 1.09 2.15 2.4 0 1.32-.95 2.4-2.15 2.4Z" />
        </svg>
      </span>
      Discord
      <span class="ext" aria-hidden="true"><ArrowUpRight size={13} /></span>
    </button>
  </div>

  <!-- A toggle or a choice commits at once; typed text when the box is left
       or Enter is pressed. See lib/autosave.js. -->
  <div
    class="tab-body"
    on:change={saveNow}
    on:keydown={(e) => e.key === 'Enter' && e.target.matches('input') && saveNow()}
    role="presentation"
  >
  {#if tab === 'general'}
    <div class="card">
      <h3 class="section-title with-icon"><Monitor size={17} />{$t('settings.general.deviceIdentity')}</h3>
      <div class="field">
        <label for="s-name">{$t('settings.general.deviceName')}</label>
        <input id="s-name" class:invalid={unsaved.includes('deviceName')} bind:value={draft.deviceName} />
      </div>
      <div class="field">
        <label for="s-type">{$t('settings.general.deviceType')}</label>
        <select id="s-type" bind:value={draft.deviceType}>
          <option value="desktop">{$t('settings.general.type.desktop')}</option>
          <option value="deck">{$t('settings.general.type.deck')}</option>
          <option value="handheld">{$t('settings.general.type.handheld')}</option>
          <option value="mobile">{$t('settings.general.type.mobile')}</option>
        </select>
        <span class="hint">{$t('settings.general.typeHint')}</span>
      </div>
      <div class="field">
        <label for="s-node">{$t('settings.general.deviceId')}</label>
        <input id="s-node" value={draft.nodeId ?? ''} readonly class="mono" />
        <span class="hint">{$t('settings.general.deviceIdHint')}</span>
      </div>
    </div>

    <!-- Kept on this device rather than in the daemon's settings, like the
         library view below. -->
    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title with-icon"><Palette size={17} />{$t('settings.general.appearance')}</h3>
      <AppearanceOptions />
    </div>

    <!-- Using OpenSave from a gamepad: a Steam Deck, a handheld, a pad on a
         PC. Also on this device only. -->
    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title with-icon"><Gamepad2 size={17} />{$t('settings.general.controller')}</h3>
      <ControllerOptions />
    </div>

    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title with-icon"><Bell size={17} />{$t('settings.general.notifications')}</h3>
      <NotificationOptions />
    </div>

    <!-- The same thing the View menu on the library changes, kept on this
         device: see lib/libraryview.js for why. -->
    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title with-icon"><LayoutGrid size={17} />{$t('settings.general.library')}</h3>
      <p class="hint library-hint">{$t('settings.general.libraryHint')}</p>
      <LibraryViewOptions />
      <div class="guide-row">
        <div>
          <strong>{$t('settings.general.setupGuide')}</strong>
          <p class="hint">{$t('settings.general.setupGuideHint')}</p>
        </div>
        <button class="btn small" on:click={() => { showSetupAgain(); navigate('home'); }}>{$t('settings.general.showSetupAgain')}</button>
      </div>
    </div>

    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title with-icon"><Rocket size={17} />{$t('settings.general.startup')}</h3>
      <label class="check">
        <input type="checkbox" bind:checked={draft.startOnBoot} />
        {$t('settings.general.startOnBoot')}
      </label>
      <p class="hint" style="margin-top: 6px;">
        {$t('settings.general.startOnBootHint')}
      </p>
    </div>

    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title with-icon"><Download size={17} />{$t('settings.general.updates')}</h3>
      <label class="check">
        <input
          type="checkbox"
          checked={draft.updateChannel === 'beta'}
          on:change={(e) => (draft.updateChannel = e.currentTarget.checked ? 'beta' : 'stable')}
        />
        {$t('settings.general.offerBetas')}
      </label>
      <p class="hint" style="margin-top: 6px;">
        {#if onPreRelease}
          {#each segments($t('settings.general.betasOnPreRelease'), { version: appVersion }, ['version']) as part}{#if part.bold}<strong>{part.text}</strong>{:else}{part.text}{/if}{/each}
        {:else}
          {$t('settings.general.betasStable')}
        {/if}
      </p>
    </div>

  {:else if tab === 'sync'}
    <div class="card">
      <h3 class="section-title with-icon"><RefreshCw size={17} />{$t('settings.sync.behavior')}</h3>
      <label class="check">
        <input type="checkbox" bind:checked={draft.autoSyncOnTrack} />
        {$t('settings.sync.autoSyncOnTrack')}
      </label>
      <label class="check" style="margin-top: 18px;">
        <input type="checkbox" bind:checked={draft.detectNewGames} />
        {$t('settings.sync.detectNewGames')}
      </label>
      <span class="hint" style="margin-top: 6px;">
        {$t('settings.sync.detectNewGamesHint')}
      </span>
      <label class="check" style="margin-top: 18px;">
        <input type="checkbox" bind:checked={draft.matchByAppId} />
        {$t('settings.sync.matchByAppId')}
      </label>
      <span class="hint" style="margin-top: 6px;">
        {$t('settings.sync.matchByAppIdHint')}
        <MoreInfo>
          {$t('settings.sync.matchByAppIdMore')}
        </MoreInfo>
      </span>
        <div class="field" style="margin-top: 18px;">
          <label for="s-unknown-game">{$t('settings.sync.unknownGameLabel')}</label>
          <select id="s-unknown-game" bind:value={draft.unknownGameFromPeer}>
            <option value="track">{$t('settings.sync.unknownGame.track')}</option>
            <option value="ask">{$t('settings.sync.unknownGame.ask')}</option>
          </select>
          <span class="hint">
            {$t('settings.sync.unknownGameHint')}
            <MoreInfo>
              {#each segments($t('settings.sync.unknownGameMore'), { askMe: $t('settings.sync.askShort') }, ['askMe']) as part}{#if part.bold}<em>{part.text}</em>{:else}{part.text}{/if}{/each}
            </MoreInfo>
          </span>
        </div>
      <div class="field" style="margin-top: 14px;">
        <label for="s-limit">{$t('settings.sync.bandwidthLimit')}</label>
        <select id="s-limit" bind:value={draft.speedLimit}>
          <option value={0}>{$t('settings.sync.speed.unlimited')}</option>
          <option value={100}>{$t('settings.sync.speed.low')}</option>
          <option value={500}>{$t('settings.sync.speed.medium')}</option>
          <option value={1024}>{$t('settings.sync.speed.high')}</option>
          <option value={5120}>{$t('settings.sync.speed.veryHigh')}</option>
          <option value={10240}>{$t('settings.sync.speed.ultra')}</option>
        </select>
        <span class="hint">{$t('settings.sync.speedHint')}</span>
      </div>
    </div>

    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title with-icon"><Globe size={17} />{$t('settings.sync.relay')}</h3>
      <div class="field">
        <label for="s-relay-url">{$t('settings.sync.relayUrl')}</label>
        <input id="s-relay-url" class:invalid={unsaved.includes('relayUrl')} bind:value={draft.relayUrl} placeholder="wss://relay.opensave.org" />
        <span class="hint">{#each segments($t('settings.sync.relayUrlHint'), { internetSync: $t('settings.sync.internetSync') }, ['internetSync']) as part}{#if part.bold}<strong>{part.text}</strong>{:else}{part.text}{/if}{/each}</span>
      </div>
      <label class="check">
        <input type="checkbox" bind:checked={draft.hostRelay} />
        {$t('settings.sync.hostRelay')}
      </label>
      <p class="hint" style="margin-top: 6px;">
        {$t('settings.sync.hostRelayHint')}
      </p>
      {#if draft.hostRelay}
        <div class="field" style="margin-top: 12px;">
          <label for="s-relay-port">{$t('settings.sync.relayPort')}</label>
          <input id="s-relay-port" type="number" class:invalid={unsaved.includes('relayPort')} bind:value={draft.relayPort} />
          <span class="hint">{$t('settings.sync.relayPortHint')}</span>
        </div>
        <button class="btn small" on:click={toggleRelayInfo} disabled={relayInfoLoading}>
          {#if relayInfoLoading}
            {$t('settings.sync.lookingUp')}
          {:else if relayInfoShown}
            {$t('settings.sync.hideAddresses')}
          {:else}
            {$t('settings.sync.showAddresses')}
          {/if}
        </button>
        {#if relayInfoShown && relayInfo}
          <div class="share-banner">
            <div class="share-title with-icon"><RadioTower size={15} />{$t('settings.sync.shareTitle')}</div>
            <div class="share-row"><span>{$t('settings.sync.lanIps')}</span> {relayInfo.lanIps?.join(', ') || '—'}</div>
            <div class="share-row"><span>{$t('settings.sync.publicIp')}</span> {relayInfo.publicIp || $t('settings.sync.unavailable')}</div>
            <div class="share-row"><span>{$t('settings.sync.relayPortLabel')}</span> {relayInfo.relayPort}</div>
          </div>
        {/if}
      {/if}
    </div>

    <div class="card moved" style="margin-top: 14px;">
      <div>
        <h3 class="section-title with-icon"><Cloud size={17} />{$t('home.cloudBackup')}</h3>
        <p class="hint">
          {$t('settings.sync.cloudBackupHint')}
        </p>
      </div>
      <button class="btn" on:click={() => navigate('cloud')}>{$t('settings.sync.openCloudBackup')}</button>
    </div>
  {:else if tab === 'storage'}
    <div class="card">
      <h3 class="section-title with-icon"><PieChart size={17} />{$t('settings.storage.spaceUsed')}</h3>
      <StorageUsage />
    </div>

    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title with-icon"><FileCheck size={17} />{$t('settings.storage.canRestore')}</h3>
      <SnapshotChecks />
    </div>

    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title with-icon"><HardDrive size={17} />{$t('settings.storage.snapshotStorage')}</h3>
      <div class="field">
        <label for="s-backups">{$t('settings.storage.snapshotsFolder')}</label>
        <div class="path-row">
          <input id="s-backups" bind:value={draft.backupsDir} />
          <button class="btn" on:click={pickBackupsDir}>{$t('settings.storage.browse')}</button>
        </div>
        <span class="hint">{$t('settings.storage.snapshotsFolderHint')}</span>
      </div>
      <div class="field">
        <label for="s-sync-backups">{$t('settings.storage.safetyFolder')}</label>
        <div class="path-row">
          <input id="s-sync-backups" bind:value={draft.syncBackupsDir} placeholder={$t('settings.storage.safetyFolderPlaceholder')} />
          <button class="btn" on:click={pickSyncBackupsDir}>{$t('settings.storage.browse')}</button>
        </div>
        <span class="hint">{$t('settings.storage.safetyFolderHint')}</span>
      </div>
    </div>

    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title with-icon"><Timer size={17} />{$t('settings.storage.retention')}</h3>
      <label class="check">
        <input type="checkbox" bind:checked={draft.autoDeleteBackups} />
        {$t('settings.storage.autoDelete')}
      </label>
      <span class="hint">
        {$t('settings.storage.autoDeleteHint')}
      </span>
      {#if draft.autoDeleteBackups}
        <div class="field" style="margin-top: 10px;">
          <label for="s-days">{$t('settings.storage.deleteOlderThan')}</label>
          <select id="s-days" bind:value={draft.autoDeleteDays}>
            <option value={7}>{$t('settings.storage.days', { n: 7 })}</option>
            <option value={14}>{$t('settings.storage.days', { n: 14 })}</option>
            <option value={30}>{$t('settings.storage.days', { n: 30 })}</option>
            <option value={60}>{$t('settings.storage.days', { n: 60 })}</option>
            <option value={90}>{$t('settings.storage.days', { n: 90 })}</option>
            <option value={180}>{$t('settings.storage.days', { n: 180 })}</option>
          </select>
        </div>
      {/if}
    </div>

    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title with-icon"><History size={17} />{$t('settings.storage.snapshotHistory')}</h3>
      <div class="field">
        <label for="s-max-snaps">{$t('settings.storage.maxAuto')}</label>
        <input id="s-max-snaps" type="number" min="0" style="max-width: 120px;" bind:value={draft.defaultMaxSnapshots} />
        <span class="hint">
          {$t('settings.storage.maxAutoHint')}
        </span>
      </div>
      <div class="field">
        <label for="s-max-manual-snaps">{$t('settings.storage.maxManual')}</label>
        <input id="s-max-manual-snaps" type="number" min="0" style="max-width: 120px;" bind:value={draft.defaultMaxManualSnapshots} />
        <span class="hint">
          {#each segments($t('settings.storage.maxManualHint'), { forever: $t('settings.storage.keepForever') }, ['forever']) as part}{#if part.bold}<strong>{part.text}</strong>{:else}{part.text}{/if}{/each}
        </span>
      </div>
      <div class="field" style="margin-bottom: 0;">
        <div>
          <button class="btn small" disabled={pruning} on:click={cleanUpSnapshots}>
            <BrushCleaning size={14} />{pruning ? $t('settings.storage.applying') : $t('settings.storage.applyLimits')}
          </button>
        </div>
        <span class="hint">
          {$t('settings.storage.applyLimitsHint')}
        </span>
      </div>
    </div>

    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title with-icon"><ScanSearch size={17} />{$t('settings.storage.scanner')}</h3>
      <div class="field" style="margin-bottom: 0;">
        <label for="s-scan-paths">{$t('settings.storage.scanPaths')}</label>
        <span class="hint">{$t('settings.storage.scanPathsHint')}</span>
        {#each draft.customScanPaths ?? [] as p, i}
          <div class="rule-row">
            <span class="rule-path" title={p}>{p}</span>
            <button class="btn small ghost icon" title={$t('settings.remove')} aria-label={$t('settings.remove')} on:click={() => removeScanPath(i)}><X size={15} /></button>
          </div>
        {/each}
        <button id="s-scan-paths" class="btn small" on:click={addScanPath}><Plus size={14} />{$t('settings.storage.addFolder')}</button>
      </div>

      <div class="field" style="margin: 18px 0 0;">
        <label for="s-exclude-paths">{$t('settings.storage.excludePaths')}</label>
        <span class="hint">{$t('settings.storage.excludePathsHint')}</span>
        {#each draft.excludePaths ?? [] as p, i}
          <div class="rule-row">
            <span class="rule-path" title={p}>{p}</span>
            <button class="btn small ghost icon" title={$t('settings.remove')} aria-label={$t('settings.remove')} on:click={() => removeExcludePath(i)}><X size={15} /></button>
          </div>
        {/each}
        <button id="s-exclude-paths" class="btn small" on:click={addExcludePath}><Plus size={14} />{$t('settings.storage.excludeFolder')}</button>
      </div>
    </div>

    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title with-icon"><RotateCcw size={17} />{$t('settings.reset.title')}</h3>
      <div class="field" style="margin-bottom: 0;">
        <span class="hint">{$t('settings.reset.hint')}</span>
        <button
          class="btn small danger"
          style="margin-top: 12px; width: fit-content; align-self: flex-start;"
          on:click={resetTracking}
          disabled={resetting || $gameList.length === 0}
        >
          {#if resetting}{$t('settings.reset.inProgress')}{:else if $gameList.length === 0}{$t('settings.reset.none')}{:else}{$t('settings.reset.button', { n: $gameList.length })}{/if}
        </button>
      </div>
    </div>
  {:else if tab === 'advanced'}
    <div class="card">
      <h3 class="section-title with-icon"><Network size={17} />{$t('settings.advanced.network')}</h3>
      <div class="field" style="margin-bottom: 0;">
        <label for="s-port">{$t('settings.advanced.daemonPort')}</label>
        <input id="s-port" type="number" class:invalid={unsaved.includes('port')} bind:value={draft.port} />
        <span class="hint">{$t('settings.advanced.daemonPortHint')}</span>
      </div>
    </div>

    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title with-icon"><ArrowLeftRight size={17} />{$t('settings.advanced.pathTranslation')}</h3>
      <div class="field" style="margin-bottom: 0;">
        <span class="hint">
          {$t('settings.advanced.pathTranslationHint')}
        </span>
        {#each draft.pathTranslations ?? [] as rule, i}
          <div class="rule-row">
            <input placeholder={$t('settings.advanced.fromPattern')} bind:value={rule.fromPattern} />
            <span class="arrow"><ArrowRight size={15} /></span>
            <input placeholder={$t('settings.advanced.toPattern')} bind:value={rule.toPattern} />
            <button class="btn small ghost icon" title={$t('settings.remove')} aria-label={$t('settings.remove')} on:click={() => removeRule(i)}><X size={15} /></button>
          </div>
        {/each}
        <button id="s-rules" class="btn small" on:click={addRule}><Plus size={14} />{$t('settings.advanced.addRule')}</button>
      </div>
    </div>
  {:else if tab === 'support'}
    <div class="card support-card">
      <div class="support-hero">
        <div class="support-badge"><Heart size={22} /></div>
        <div class="support-hero-text">
          <h3 class="support-title">{$t('settings.support.title')}</h3>
          <p class="support-lede">
            {$t('settings.support.lede')}
          </p>
        </div>
      </div>

      <div class="support-split">
        <div class="support-left">
          <p class="support-body">
            {$t('settings.support.body')}
          </p>
          <ul class="support-points">
            <li><span class="pt-icon"><Globe size={15} /></span> {$t('settings.support.point1')}</li>
            <li><span class="pt-icon"><Gamepad2 size={15} /></span> {$t('settings.support.point2')}</li>
            <li><span class="pt-icon"><Wrench size={15} /></span> {$t('settings.support.point3')}</li>
          </ul>

          <div class="support-actions">
            <button class="btn primary support-cta" on:click={openDonatePage}>
              {donateOpened ? $t('settings.support.openAgain') : $t('settings.support.openDonate')}<ArrowUpRight size={15} />
            </button>
            {#if donateOpened}
              <span class="support-opened">{$t('settings.support.openedToast')}</span>
            {/if}
          </div>
          <p class="support-foot">
            {$t('settings.support.foot')}
          </p>
        </div>

        <div class="support-qr">
          <!-- Rendered locally from DONATE_URL; no network request, no external
               image service. Kept on a light plate because phone cameras
               struggle with inverted (light-on-dark) QR codes. -->
          <div class="qr-plate">{@html qrSvg}</div>
          <span class="qr-caption">{$t('settings.support.scanToPay')}<br />{$t('settings.support.fromPhone')}</span>
        </div>
      </div>

      <p class="support-note">
        {$t('settings.support.note')}
      </p>
    </div>
  {/if}

  </div>
{/if}

<style>
  .head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 16px;
    margin-bottom: 18px;
  }
  .save-state {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: 0.8rem;
    color: var(--text-faint);
    min-width: 0;
  }
  .save-state.error {
    color: var(--danger-text);
  }
  .why {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: min(560px, 60vw);
  }
  input.invalid {
    border-color: var(--danger);
  }
  .save-state :global(.spin) {
    animation: settings-spin 1s linear infinite;
  }
  @keyframes settings-spin {
    to {
      transform: rotate(360deg);
    }
  }
  /* .check and the checkbox itself are styled globally in app.css. */
  .path-row {
    display: flex;
    gap: 8px;
  }
  .path-row input {
    flex: 1;
  }
  .rule-row {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 8px;
  }
  .rule-row input {
    flex: 1;
    padding: 7px 10px;
    background: var(--bg);
    border: 1px solid var(--border-strong);
    border-radius: 8px;
    color: var(--text);
    font-size: 0.85rem;
    outline: none;
  }
  .rule-path {
    flex: 1;
    font-size: 0.83rem;
    color: var(--text-dim);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .arrow {
    color: var(--text-faint);
  }
  /* Support tab: set apart from the settings tabs (it configures nothing) but
     never shouty — no accent fill, just a softer separated pill. */
  .support-tab {
    margin-left: auto;
  }
  /* Discord's own blurple, so it is recognisable at a glance, but kept at
     the same weight as the tabs beside it rather than shouting over them. */
  .pill-tabs .discord-tab {
    color: #9ba1f7;
  }
  .pill-tabs .discord-tab:hover {
    color: #fff;
    background: rgba(88, 101, 242, 0.35);
  }
  :global(:root[data-theme='light']) .pill-tabs .discord-tab {
    color: #4752c4;
  }
  :global(:root[data-theme='light']) .pill-tabs .discord-tab:hover {
    color: #fff;
    background: #5865f2;
  }
  .discord-glyph {
    display: inline-flex;
  }
  .ext {
    display: inline-flex;
    opacity: 0.7;
  }
  /* This is the one tab that isn't a settings form, so it carries a little
     accent identity instead of reading as another block of options. */
  .support-hero {
    display: flex;
    align-items: center;
    gap: 16px;
    margin-bottom: 22px;
    padding: 18px 20px;
    border-radius: var(--radius-lg);
    background:
      radial-gradient(120% 160% at 0% 0%, var(--accent-soft), transparent 62%),
      var(--bg);
    border: 1px solid var(--border);
  }
  .support-badge {
    flex: none;
    width: 46px;
    height: 46px;
    display: grid;
    place-items: center;
    color: var(--accent-text);
    border-radius: 50%;
    background: var(--accent-soft);
    border: 1px solid rgba(var(--accent-rgb), 0.35);
  }
  .support-hero-text {
    min-width: 0;
  }
  .support-title {
    font-size: 1.05rem;
    font-weight: 600;
    margin: 0 0 5px;
  }
  .support-lede {
    font-size: 0.88rem;
    color: var(--text-dim);
    margin: 0;
    line-height: 1.5;
    max-width: 68ch;
  }
  .support-body {
    font-size: 0.88rem;
    color: var(--text-dim);
    margin: 0 0 16px;
    line-height: 1.5;
    max-width: 56ch;
  }
  .support-points {
    list-style: none;
    margin: 0 0 22px;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .support-points li {
    display: flex;
    align-items: center;
    gap: 11px;
    font-size: 0.86rem;
    color: var(--text);
  }
  .pt-icon {
    flex: none;
    width: 28px;
    height: 28px;
    display: grid;
    place-items: center;
    color: var(--text-dim);
    border-radius: 8px;
    background: var(--bg-hover);
    border: 1px solid var(--border);
  }
  .support-actions {
    display: flex;
    gap: 12px;
    align-items: center;
    flex-wrap: wrap;
  }
  .support-cta {
    padding: 9px 18px;
  }
  .support-foot {
    font-size: 0.8rem;
    color: var(--text-faint);
    margin: 16px 0 0;
  }
  .support-split {
    display: flex;
    gap: 32px;
    align-items: flex-start;
    flex-wrap: wrap;
  }
  .support-left {
    flex: 1;
    min-width: 260px;
  }
  .support-opened {
    font-size: 0.82rem;
    color: var(--text-dim);
  }
  /* Framed panel so the code reads as a deliberate element rather than an
     image floating in empty space. */
  .support-qr {
    flex: none;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 10px;
    padding: 16px;
    border-radius: var(--radius-lg);
    background: var(--bg);
    border: 1px solid var(--border);
  }
  /* Light plate: phone cameras are unreliable at reading inverted QR codes,
     so the code stays dark-on-light even in the dark theme. */
  .qr-plate {
    background: #fff;
    padding: 10px;
    border-radius: 10px;
    line-height: 0;
  }
  .qr-plate :global(svg) {
    display: block;
    width: 124px;
    height: 124px;
    shape-rendering: crispEdges;
  }
  .qr-caption {
    font-size: 0.76rem;
    color: var(--text-faint);
    text-align: center;
    line-height: 1.4;
  }
  .support-note {
    font-size: 0.78rem;
    color: var(--text-faint);
    margin: 18px 0 0;
    padding-top: 12px;
    border-top: 1px solid var(--border);
  }
  /* A pointer to where the cloud settings moved, rather than a gap where
     they were. */
  .moved {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
  }
  .moved .hint {
    font-size: 0.82rem;
    color: var(--text-faint);
    line-height: 1.5;
  }
  .guide-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    margin-top: 18px;
    padding-top: 14px;
    border-top: 1px solid var(--border);
  }
  .guide-row strong {
    font-size: 0.88rem;
  }
  .guide-row .hint {
    margin: 2px 0 0;
  }
  .library-hint {
    font-size: 0.82rem;
    color: var(--text-faint);
    margin: -4px 0 14px;
  }
  .section-title {
    font-size: 0.95rem;
    font-weight: 600;
    margin-bottom: 14px;
    padding-bottom: 10px;
    border-bottom: 1px solid var(--border);
    color: var(--text);
  }
  .mono {
    font-family: ui-monospace, 'Cascadia Code', 'Consolas', monospace;
    font-size: 0.82rem;
    color: var(--text-dim);
  }
  input[readonly] {
    opacity: 0.75;
    cursor: default;
  }
  .share-banner {
    margin-top: 12px;
    background: rgba(var(--accent-rgb), 0.06);
    border: 1px solid rgba(var(--accent-rgb), 0.28);
    border-radius: var(--radius);
    padding: 12px 14px;
    font-size: 0.82rem;
  }
  .share-title {
    font-weight: 600;
    margin-bottom: 6px;
  }
  .share-row {
    color: var(--text-dim);
    margin-top: 3px;
  }
  .share-row span {
    color: var(--text-faint);
    display: inline-block;
    width: 78px;
  }
</style>
