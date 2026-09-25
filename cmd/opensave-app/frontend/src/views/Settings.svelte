<script>
  import { onMount, onDestroy } from 'svelte';
  import { settings, toast, askConfirm, gameList, navigate } from '../lib/stores.js';
  import { api, native } from '../lib/api.js';
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
    relayUrl: 'relay URL',
    relayPort: 'relay hosting port',
    port: 'daemon port',
    deviceName: 'device name',
    backupsDir: 'snapshots folder',
    syncBackupsDir: 'safety backups folder',
    pathTranslations: 'path rules'
  };
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
      if (saveState?.error) throw new Error(`the limits couldn't be saved: ${saveState.error}`);
      const res = await api.post('/api/snapshots/prune', { applyDefaultToAll: true });
      const mb = (res.freedBytes / 1048576).toFixed(1);
      toast(
        res.removed > 0
          ? `Removed ${res.removed} old snapshot${res.removed === 1 ? '' : 's'}, freed ${mb} MB`
          : 'Nothing to clean up — all games are within their limit',
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
    const dir = await native.selectDirectory('Add a folder to auto-scan');
    if (dir) draft.customScanPaths = [...(draft.customScanPaths ?? []), dir];
    saveNow();
  }
  function removeScanPath(i) {
    draft.customScanPaths = draft.customScanPaths.filter((_, idx) => idx !== i);
    saveNow();
  }

  // Excluded folders — locations the auto-scan should skip entirely.
  async function addExcludePath() {
    const dir = await native.selectDirectory('Choose a folder to exclude from auto-scan');
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
    const ok = await askConfirm(
      `Untrack all ${n} game${n === 1 ? '' : 's'}? They'll be removed from your library so you can re-add them from the correct locations. Your save snapshots on disk are kept — nothing is deleted.`,
      { title: 'Reset tracking?', confirmText: `Untrack ${n}`, danger: true }
    );
    if (!ok) return;
    resetting = true;
    try {
      const res = await api.post('/api/games/untrack-bulk', { all: true });
      toast(`Untracked ${res.untracked} game${res.untracked === 1 ? '' : 's'} — snapshots kept`, 'success');
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      resetting = false;
    }
  }

  async function pickBackupsDir() {
    const dir = await native.selectDirectory('Select snapshots storage folder');
    if (dir) draft.backupsDir = dir;
    saveNow();
  }

  async function pickSyncBackupsDir() {
    const dir = await native.selectDirectory('Select pre-sync safety backups folder');
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
  <h2 class="page-title">Settings</h2>
  {#if draft}
    <span class="save-state" class:error={saveState?.error || unsaved.length} aria-live="polite">
      {#if saveState === 'saving'}
        <LoaderCircle size={14} class="spin" />Saving…
      {:else if unsaved.length}
        <TriangleAlert size={14} />
        <span class="why" title={refusedWhy[unsaved[0]]}>
          Not saved: the {FIELD_NAMES[unsaved[0]] ?? unsaved[0]} — {firstSentence(refusedWhy[unsaved[0]])}
        </span>
      {:else if saveState?.error}
        <TriangleAlert size={14} /><span class="why" title={saveState.error}>Couldn't save: {firstSentence(saveState.error)}</span>
      {:else if saveState === 'saved'}
        <Check size={14} />Saved
      {:else}
        Changes save as you make them
      {/if}
    </span>
  {/if}
</div>

{#if !draft}
  <Skeleton kind="cards" count={3} />
{:else}
  <div class="pill-tabs" style="margin-bottom: 18px;">
    <button class:active={tab === 'general'} on:click={() => showTab('general')}>General</button>
    <button class:active={tab === 'sync'} on:click={() => showTab('sync')}>Sync</button>
    <button class:active={tab === 'storage'} on:click={() => showTab('storage')}>Storage</button>
    <button class:active={tab === 'advanced'} on:click={() => showTab('advanced')}>Advanced</button>
    <button class="support-tab" class:active={tab === 'support'} on:click={() => showTab('support')}><Heart size={14} />Support</button>
    <!-- Not a tab: it leaves the app. Shaped like its neighbour so the pair
         reads as one group, marked with an outward arrow so nobody expects a panel. -->
    <button class="discord-tab" on:click={() => native.openExternal(DISCORD_URL)} title="Open the OpenSave Discord in your browser">
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
      <h3 class="section-title with-icon"><Monitor size={17} />Device identity</h3>
      <div class="field">
        <label for="s-name">Device name — how other devices see you</label>
        <input id="s-name" class:invalid={unsaved.includes('deviceName')} bind:value={draft.deviceName} />
      </div>
      <div class="field">
        <label for="s-type">Device type</label>
        <select id="s-type" bind:value={draft.deviceType}>
          <option value="desktop">Desktop (Windows / macOS / Linux PC)</option>
          <option value="deck">Steam Deck (SteamOS handheld)</option>
          <option value="handheld">Handheld (ROG Ally / Legion Go / emulator)</option>
          <option value="mobile">Companion (mobile device)</option>
        </select>
        <span class="hint">Shown to other devices when they discover you.</span>
      </div>
      <div class="field">
        <label for="s-node">Device ID</label>
        <input id="s-node" value={draft.nodeId ?? ''} readonly class="mono" />
        <span class="hint">This device's unique network identifier (read-only).</span>
      </div>
    </div>

    <!-- Kept on this device rather than in the daemon's settings, like the
         library view below. -->
    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title with-icon"><Palette size={17} />Appearance</h3>
      <AppearanceOptions />
    </div>

    <!-- Using OpenSave from a gamepad: a Steam Deck, a handheld, a pad on a
         PC. Also on this device only. -->
    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title with-icon"><Gamepad2 size={17} />Controller</h3>
      <ControllerOptions />
    </div>

    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title with-icon"><Bell size={17} />Notifications</h3>
      <NotificationOptions />
    </div>

    <!-- The same thing the View menu on the library changes, kept on this
         device: see lib/libraryview.js for why. -->
    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title with-icon"><LayoutGrid size={17} />Library</h3>
      <p class="hint library-hint">How your games are laid out on Home. Changes apply straight away.</p>
      <LibraryViewOptions />
    </div>

    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title with-icon"><Rocket size={17} />Startup</h3>
      <label class="check">
        <input type="checkbox" bind:checked={draft.startOnBoot} />
        Start OpenSave when the computer starts
      </label>
      <p class="hint" style="margin-top: 6px;">
        Launches minimized to the system tray so syncing runs in the background.
      </p>
    </div>

    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title with-icon"><Download size={17} />Updates</h3>
      <label class="check">
        <input
          type="checkbox"
          checked={draft.updateChannel === 'beta'}
          on:change={(e) => (draft.updateChannel = e.currentTarget.checked ? 'beta' : 'stable')}
        />
        Offer me beta versions
      </label>
      <p class="hint" style="margin-top: 6px;">
        {#if onPreRelease}
          You're running <strong>{appVersion}</strong>, which is a beta, so you'll be offered newer
          betas whether or not this is ticked — otherwise there'd be no way forward until the final
          release caught up. Untick it and you'll move to the stable release as soon as it's newer
          than what you have.
        {:else}
          Betas ship early with the newest fixes and are less tested. You'll be offered the stable
          release whenever it's newer, so this isn't a one-way door.
        {/if}
      </p>
    </div>

  {:else if tab === 'sync'}
    <div class="card">
      <h3 class="section-title with-icon"><RefreshCw size={17} />Sync behavior</h3>
      <label class="check">
        <input type="checkbox" bind:checked={draft.autoSyncOnTrack} />
        Sync a game immediately when it's first tracked
      </label>
      <label class="check" style="margin-top: 18px;">
        <input type="checkbox" bind:checked={draft.detectNewGames} />
        Tell me when a game turns up with saves OpenSave isn't keeping
      </label>
      <span class="hint" style="margin-top: 6px;">
        OpenSave looks every hour, in the background, and only mentions games it hasn't seen before.
        Nothing is tracked until you choose it.
      </span>
      <label class="check" style="margin-top: 18px;">
        <input type="checkbox" bind:checked={draft.matchByAppId} />
        Match saves across PCs by Steam App ID
      </label>
      <span class="hint" style="margin-top: 6px;">
        Links the same game across devices even when it was tracked under different names or drives.
        <MoreInfo>
          For example a Steam copy on one PC and a standalone copy on another. Leave this off if you
          deliberately keep two separate copies of the same game that shouldn't merge. You can always
          link games by hand from a game's page.
        </MoreInfo>
      </span>
        <div class="field" style="margin-top: 18px;">
          <label for="s-unknown-game">When another device syncs a game this one doesn't have</label>
          <select id="s-unknown-game" bind:value={draft.unknownGameFromPeer}>
            <option value="track">Start tracking it automatically (recommended)</option>
            <option value="ask">Ask me where to keep it</option>
          </select>
          <span class="hint">
            Tracking automatically works out a folder from where the game lives on the other
            device, which is why OpenSave usually needs no setup.
            <MoreInfo>
              Choose <em>Ask me</em> if your saves are somewhere that guess would get wrong — a
              second drive, a folder you moved, or a game that keeps saves in a folder named after
              your account. Games waiting for a folder appear on the Home page.
            </MoreInfo>
          </span>
        </div>
      <div class="field" style="margin-top: 14px;">
        <label for="s-limit">Internet bandwidth limit</label>
        <select id="s-limit" bind:value={draft.speedLimit}>
          <option value={0}>Unlimited (max speed)</option>
          <option value={100}>100 KB/s (very low)</option>
          <option value={500}>500 KB/s (medium)</option>
          <option value={1024}>1 MB/s (high)</option>
          <option value={5120}>5 MB/s (very high)</option>
          <option value={10240}>10 MB/s (ultra)</option>
        </select>
        <span class="hint">Only applies to relay (internet) syncs — LAN is never throttled.</span>
      </div>
    </div>

    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title with-icon"><Globe size={17} />Internet relay</h3>
      <div class="field">
        <label for="s-relay-url">WebSocket relay URL</label>
        <input id="s-relay-url" class:invalid={unsaved.includes('relayUrl')} bind:value={draft.relayUrl} placeholder="wss://relay.opensave.org" />
        <span class="hint">The relay that carries syncs across the internet. Join a room from <strong>Internet Sync</strong>.</span>
      </div>
      <label class="check">
        <input type="checkbox" bind:checked={draft.hostRelay} />
        Host a WAN relay server on this device
      </label>
      <p class="hint" style="margin-top: 6px;">
        Lets friends connect directly to you instead of the public relay.
      </p>
      {#if draft.hostRelay}
        <div class="field" style="margin-top: 12px;">
          <label for="s-relay-port">Relay hosting port</label>
          <input id="s-relay-port" type="number" class:invalid={unsaved.includes('relayPort')} bind:value={draft.relayPort} />
          <span class="hint">Forward this TCP port on your router so friends on the internet can reach you.</span>
        </div>
        <button class="btn small" on:click={toggleRelayInfo} disabled={relayInfoLoading}>
          {#if relayInfoLoading}
            Looking up…
          {:else if relayInfoShown}
            Hide my addresses
          {:else}
            Show my addresses to share
          {/if}
        </button>
        {#if relayInfoShown && relayInfo}
          <div class="share-banner">
            <div class="share-title with-icon"><RadioTower size={15} />Share these with your friend</div>
            <div class="share-row"><span>LAN IPs:</span> {relayInfo.lanIps?.join(', ') || '—'}</div>
            <div class="share-row"><span>Public IP:</span> {relayInfo.publicIp || 'unavailable'}</div>
            <div class="share-row"><span>Relay port:</span> {relayInfo.relayPort}</div>
          </div>
        {/if}
      {/if}
    </div>

    <div class="card moved" style="margin-top: 14px;">
      <div>
        <h3 class="section-title with-icon"><Cloud size={17} />Cloud backup</h3>
        <p class="hint">
          Where backups go, whether every snapshot is sent there, and whether newer saves are
          brought from your other devices are all set on the Cloud Backup page.
        </p>
      </div>
      <button class="btn" on:click={() => navigate('cloud')}>Open Cloud Backup</button>
    </div>
  {:else if tab === 'storage'}
    <div class="card">
      <h3 class="section-title with-icon"><PieChart size={17} />Space used</h3>
      <StorageUsage />
    </div>

    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title with-icon"><FileCheck size={17} />Can they be restored?</h3>
      <SnapshotChecks />
    </div>

    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title with-icon"><HardDrive size={17} />Snapshot storage</h3>
      <div class="field">
        <label for="s-backups">Snapshots folder</label>
        <div class="path-row">
          <input id="s-backups" bind:value={draft.backupsDir} />
          <button class="btn" on:click={pickBackupsDir}>Browse</button>
        </div>
        <span class="hint">Where version-history snapshots (ZIP archives) are stored.</span>
      </div>
      <div class="field">
        <label for="s-sync-backups">Pre-sync safety backups folder</label>
        <div class="path-row">
          <input id="s-sync-backups" bind:value={draft.syncBackupsDir} placeholder="Default: ~/.opensave/backups" />
          <button class="btn" on:click={pickSyncBackupsDir}>Browse</button>
        </div>
        <span class="hint">A safety copy of your save is taken here before every incoming sync, so a bad sync is always reversible.</span>
      </div>
    </div>

    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title with-icon"><Timer size={17} />Retention</h3>
      <label class="check">
        <input type="checkbox" bind:checked={draft.autoDeleteBackups} />
        Auto-delete old automatic snapshots
      </label>
      <span class="hint">
        The snapshots OpenSave takes on its own — before a sync replaces files, when a game saves, at a
        conflict. Snapshots you took yourself are kept, and so is the newest one on every branch, whatever
        its age. Runs shortly after start and every few hours.
      </span>
      {#if draft.autoDeleteBackups}
        <div class="field" style="margin-top: 10px;">
          <label for="s-days">Delete when older than</label>
          <select id="s-days" bind:value={draft.autoDeleteDays}>
            <option value={7}>7 days</option>
            <option value={14}>14 days</option>
            <option value={30}>30 days</option>
            <option value={60}>60 days</option>
            <option value={90}>90 days</option>
            <option value={180}>180 days</option>
          </select>
        </div>
      {/if}
    </div>

    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title with-icon"><History size={17} />Snapshot history</h3>
      <div class="field">
        <label for="s-max-snaps">Automatic snapshots to keep per game</label>
        <input id="s-max-snaps" type="number" min="0" style="max-width: 120px;" bind:value={draft.defaultMaxSnapshots} />
        <span class="hint">
          Default limit for newly tracked games (0 = keep everything). The oldest are pruned first,
          per branch. Change a single game's limit in its Configuration tab.
        </span>
      </div>
      <div class="field">
        <label for="s-max-manual-snaps">Manual snapshots to keep per game</label>
        <input id="s-max-manual-snaps" type="number" min="0" style="max-width: 120px;" bind:value={draft.defaultMaxManualSnapshots} />
        <span class="hint">
          Snapshots you take yourself have their own budget, so games that auto-save often
          (Elden Ring, Dragonsword) can't push them out. <strong>0 = keep forever</strong>, which is
          the default.
        </span>
      </div>
      <div class="field" style="margin-bottom: 0;">
        <div>
          <button class="btn small" disabled={pruning} on:click={cleanUpSnapshots}>
            <BrushCleaning size={14} />{pruning ? 'Applying…' : 'Apply these limits to every game'}
          </button>
        </div>
        <span class="hint">
          Replaces each game's own limits with these, then deletes the snapshots beyond them on
          every branch. To clean up with each game's own limits, use Space used above.
        </span>
      </div>
    </div>

    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title with-icon"><ScanSearch size={17} />Game scanner</h3>
      <div class="field" style="margin-bottom: 0;">
        <label for="s-scan-paths">Extra folders to auto-scan</label>
        <span class="hint">Auto-scan already checks Steam and common emulators — add custom libraries here.</span>
        {#each draft.customScanPaths ?? [] as p, i}
          <div class="rule-row">
            <span class="rule-path" title={p}>{p}</span>
            <button class="btn small ghost icon" title="Remove" aria-label="Remove" on:click={() => removeScanPath(i)}><X size={15} /></button>
          </div>
        {/each}
        <button id="s-scan-paths" class="btn small" on:click={addScanPath}><Plus size={14} />Add folder</button>
      </div>

      <div class="field" style="margin: 18px 0 0;">
        <label for="s-exclude-paths">Folders to exclude</label>
        <span class="hint">Auto-scan skips these folders and everything inside them — handy for stale save locations (like an old GSE saves directory) you don't want offered again.</span>
        {#each draft.excludePaths ?? [] as p, i}
          <div class="rule-row">
            <span class="rule-path" title={p}>{p}</span>
            <button class="btn small ghost icon" title="Remove" aria-label="Remove" on:click={() => removeExcludePath(i)}><X size={15} /></button>
          </div>
        {/each}
        <button id="s-exclude-paths" class="btn small" on:click={addExcludePath}><Plus size={14} />Exclude folder</button>
      </div>
    </div>

    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title with-icon"><RotateCcw size={17} />Reset tracking</h3>
      <div class="field" style="margin-bottom: 0;">
        <span class="hint">Untrack every game at once, then re-run Auto-scan to add them back from the correct locations — useful after moving games between launchers or drives. This only clears the tracking list; your save snapshots on disk are kept.</span>
        <button
          class="btn small danger"
          style="margin-top: 12px; width: fit-content; align-self: flex-start;"
          on:click={resetTracking}
          disabled={resetting || $gameList.length === 0}
        >
          {#if resetting}Untracking…{:else if $gameList.length === 0}No games tracked{:else}Untrack all {$gameList.length} game{$gameList.length === 1 ? '' : 's'}{/if}
        </button>
      </div>
    </div>
  {:else if tab === 'advanced'}
    <div class="card">
      <h3 class="section-title with-icon"><Network size={17} />Network</h3>
      <div class="field" style="margin-bottom: 0;">
        <label for="s-port">Daemon port</label>
        <input id="s-port" type="number" class:invalid={unsaved.includes('port')} bind:value={draft.port} />
        <span class="hint">The local API + LAN peer port. Changing it requires a restart.</span>
      </div>
    </div>

    <div class="card" style="margin-top: 14px;">
      <h3 class="section-title with-icon"><ArrowLeftRight size={17} />Cross-platform path translation</h3>
      <div class="field" style="margin-bottom: 0;">
        <span class="hint">
          Rewrites a peer's save paths to local conventions, e.g. "C:\Users\me\Saves" → "/home/deck/saves".
        </span>
        {#each draft.pathTranslations ?? [] as rule, i}
          <div class="rule-row">
            <input placeholder="From pattern" bind:value={rule.fromPattern} />
            <span class="arrow"><ArrowRight size={15} /></span>
            <input placeholder="To pattern" bind:value={rule.toPattern} />
            <button class="btn small ghost icon" title="Remove" aria-label="Remove" on:click={() => removeRule(i)}><X size={15} /></button>
          </div>
        {/each}
        <button id="s-rules" class="btn small" on:click={addRule}><Plus size={14} />Add rule</button>
      </div>
    </div>
  {:else if tab === 'support'}
    <div class="card support-card">
      <div class="support-hero">
        <div class="support-badge"><Heart size={22} /></div>
        <div class="support-hero-text">
          <h3 class="support-title">Support OpenSave</h3>
          <p class="support-lede">
            Free and open source, and it stays that way — no accounts, no ads, no telemetry,
            and nothing locked behind a payment.
          </p>
        </div>
      </div>

      <div class="support-split">
        <div class="support-left">
          <p class="support-body">
            It's built and maintained in spare time. If it's saved you the hassle of copying
            save folders between machines, you're welcome to chip in.
          </p>
          <ul class="support-points">
            <li><span class="pt-icon"><Globe size={15} /></span> Keeps the public relay online</li>
            <li><span class="pt-icon"><Gamepad2 size={15} /></span> More games and emulators detected</li>
            <li><span class="pt-icon"><Wrench size={15} /></span> Time for fixes and new features</li>
          </ul>

          <div class="support-actions">
            <button class="btn primary support-cta" on:click={openDonatePage}>
              {donateOpened ? 'Open again' : 'Open donation page'}<ArrowUpRight size={15} />
            </button>
            {#if donateOpened}
              <span class="support-opened">Opened in your browser — thank you.</span>
            {/if}
          </div>
          <p class="support-foot">
            Prefer not to donate? Reporting a bug or suggesting a feature helps just as much.
          </p>
        </div>

        <div class="support-qr">
          <!-- Rendered locally from DONATE_URL; no network request, no external
               image service. Kept on a light plate because phone cameras
               struggle with inverted (light-on-dark) QR codes. -->
          <div class="qr-plate">{@html qrSvg}</div>
          <span class="qr-caption">Scan to pay<br />from your phone</span>
        </div>
      </div>

      <p class="support-note">
        Payment is handled entirely by Gumroad — OpenSave never sees your card details.
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
    color: var(--accent);
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
