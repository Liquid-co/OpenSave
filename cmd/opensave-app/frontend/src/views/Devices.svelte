<script>
  // Devices: this one, the ones paired with it, the ones nearby not paired
  // yet, and internet sync. A card per paired device says where it is, the
  // games the two of you share and what moved lately (devices/DeviceCard).
  // Adding one is a dialog of its own (AddDeviceDialog).
  import { onDestroy, onMount } from 'svelte';
  import Plus from 'lucide-svelte/icons/plus';
  import Wifi from 'lucide-svelte/icons/wifi';
  import Gamepad2 from 'lucide-svelte/icons/gamepad-2';
  import Monitor from 'lucide-svelte/icons/monitor';
  import Pencil from 'lucide-svelte/icons/pencil';
  import { peers, discoveredPeers, settings, gameList, toast, askConfirm, activityTick } from '../lib/stores.js';
  import { api, native } from '../lib/api.js';
  import { isHandheld } from '../lib/devices.js';
  import { gatedFlip, pop, rise } from '../lib/motion.js';
  import { t } from '../lib/i18n.js';
  import DeviceCard from './devices/DeviceCard.svelte';
  import InternetSyncCard from './devices/InternetSyncCard.svelte';
  import AddDeviceDialog from '../components/AddDeviceDialog.svelte';
  import CheckDigits from '../components/ui/CheckDigits.svelte';
  import { sentPairings, pairClock, askToPair, waitingOn } from '../lib/pairing.js';

  export let params = {};

  let adding = params.tab === 'internet' ? 'code' : '';
  let busy = false;

  $: pairedList = Object.values($peers).sort(
    (a, b) => Number(b.status === 'online') - Number(a.status === 'online') || a.name.localeCompare(b.name)
  );
  $: pairedIds = new Set(pairedList.map((p) => p.id));
  $: nearby = $discoveredPeers.filter((d) => !d.isWan && !pairedIds.has(d.id));

  // What moved between this device and each other one lately.
  let items = [];
  async function loadItems() {
    try {
      items = (await api.get('/api/activity?limit=200')).items ?? [];
    } catch {
      items = [];
    }
  }
  let pending = null;
  const stop = activityTick.subscribe((n) => {
    clearTimeout(pending);
    pending = setTimeout(loadItems, n ? 700 : 0);
  });

  let version = '';
  let address = '';
  onMount(async () => {
    try {
      version = (await native.appInfo())?.version ?? '';
    } catch {}
    try {
      const ips = await api.get('/api/relay/ips');
      const ip = (ips.lanIps ?? []).find((a) => a.startsWith('192.168.') || a.startsWith('10.')) ?? ips.lanIps?.[0];
      if (ip) address = `${ip}:${$settings?.port ?? 8383}`;
    } catch {}
  });
  onDestroy(() => {
    stop();
    clearTimeout(pending);
  });

  async function run(fn, ok) {
    if (busy) return;
    busy = true;
    try {
      await fn();
      if (ok) toast(ok, 'success');
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      busy = false;
    }
  }

  // ── This device ──
  let renaming = false;
  let nameDraft = '';
  function startRename() {
    nameDraft = $settings?.deviceName ?? '';
    renaming = true;
  }
  const saveName = () =>
    run(async () => {
      const name = nameDraft.trim();
      if (name && name !== $settings?.deviceName) settings.set(await api.post('/api/settings', { deviceName: name }));
      renaming = false;
    });

  // ── A paired device ──
  const pairNearby = (d) => run(() => askToPair(d.id, d.deviceName, { address: d.address, port: d.port }), $t('devices.pairingSentTo', { name: d.deviceName }));
  $: sentTo = (id) => waitingOn($sentPairings, id, $pairClock);
  async function unpair(peer) {
    if (!(await askConfirm($t('devices.unpairConfirmBody', { name: peer.name }), { title: $t('devices.unpairConfirmTitle'), confirmText: $t('devices.unpairConfirmText'), danger: true }))) return;
    run(() => api.del(`/api/peers/${peer.id}`), $t('devices.unpaired', { name: peer.name }));
  }
  async function repair(peer) {
    if (!(await askConfirm($t('devices.repairConfirmBody', { name: peer.name }), { title: $t('devices.repairConfirmTitle'), confirmText: $t('devices.repairConfirmText') }))) return;
    run(() => api.del(`/api/peers/${peer.id}`), $t('devices.repairedToast', { name: peer.name }));
  }
  async function trust(peer, box) {
    const allowed = box.checked;
    if (allowed) {
      const ok = await askConfirm($t('devices.trustConfirmBody', { name: peer.name }), { title: $t('devices.trustConfirmTitle', { name: peer.name }), confirmText: $t('devices.trustConfirmText') });
      if (!ok) {
        box.checked = false;
        return;
      }
    }
    run(() => api.post(`/api/peers/${peer.id}/choose-folders`, { allowed }), allowed ? $t('devices.canChooseFolders', { name: peer.name }) : $t('devices.noLongerChooses', { name: peer.name }));
  }
  async function updateFrom(peer) {
    const ok = await askConfirm($t('devices.updateConfirmBody', { name: peer.name, version: peer.appVersion }), { title: $t('devices.updateConfirmTitle'), confirmText: $t('devices.updateConfirmText') });
    if (!ok) return;
    const err = await native.installFromPeer(peer.id);
    if (err) toast(err, 'error');
  }
</script>

<div class="head">
  <div>
    <h1 class="page-title">{$t('devices.title')}</h1>
    <p class="sub">{$t('devices.page.sub')}</p>
  </div>
  <button class="btn primary big" on:click={() => (adding = 'nearby')}><Plus size={18} />{$t('devices.add.title')}</button>
</div>

<div class="layout">
  <div class="main-col">
    <section class="card here" aria-label={$t('sidebar.thisDevice')}>
      <span class="tile"><svelte:component this={isHandheld($settings?.deviceType) ? Gamepad2 : Monitor} size={22} /></span>
      <div class="here-who">
        {#if renaming}
          <form class="rename" on:submit|preventDefault={saveName}>
            <!-- svelte-ignore a11y-autofocus -->
            <input bind:value={nameDraft} maxlength="40" autofocus aria-label={$t('devices.page.renameLabel')} on:keydown={(e) => e.key === 'Escape' && (renaming = false)} />
            <button class="btn small primary" disabled={busy || !nameDraft.trim()}>{$t('common.save')}</button>
            <button type="button" class="btn small ghost" on:click={() => (renaming = false)}>{$t('common.cancel')}</button>
          </form>
        {:else}
          <div class="here-name">{$settings?.deviceName ?? '…'} <span class="cap">{$t('sidebar.thisDevice')}</span></div>
          <div class="cap">{$t(isHandheld($settings?.deviceType) ? 'devices.page.handheld' : 'devices.page.computer')}{#if version}, OpenSave {version}{/if}</div>
        {/if}
      </div>
      <dl class="here-facts">
        {#if address}<div><dt>{$t('devices.page.reachAt')}</dt><dd class="mono">{address}</dd></div>{/if}
        <div><dt>{$t('devices.page.tracks')}</dt><dd>{$t('devices.page.games', { n: $gameList.length })}</dd></div>
      </dl>
      {#if !renaming}<button class="btn small" on:click={startRename}><Pencil size={14} />{$t('devices.page.rename')}</button>{/if}
    </section>

    {#if pairedList.length === 0}
      <div class="card empty-paired" in:rise={{ y: 6 }}>
        <h3>{$t('devices.noPaired')}</h3>
        <p>{$t('devices.noPairedHint')}</p>
        <button class="btn primary" on:click={() => (adding = 'nearby')}><Plus size={16} />{$t('devices.add.title')}</button>
      </div>
    {:else}
      <section class="grid" aria-label={$t('devices.paired')}>
        {#each pairedList as peer (peer.id)}
          <div class="cell" animate:gatedFlip in:pop={{ start: 0.96 }}>
            <DeviceCard
              {peer}
              {items}
              myVersion={version}
              {busy}
              on:unpair={() => unpair(peer)}
              on:repair={() => repair(peer)}
              on:update={() => updateFrom(peer)}
              on:trust={(e) => trust(peer, e.detail)}
            />
          </div>
        {/each}
      </section>
    {/if}
  </div>

  <aside class="side" aria-label={$t('devices.page.moreWays')}>
    <section class="card nearby" aria-labelledby="nearby-title">
      <div class="side-head"><Wifi size={17} /><h2 id="nearby-title">{$t('devices.page.nearby')}</h2></div>
      {#each nearby.slice(0, 4) as d (d.id)}
        {@const sent = sentTo(d.id)}
        <div class="near" in:rise={{ y: 4 }}>
          <span class="near-tile"><svelte:component this={isHandheld(d.deviceType) ? Gamepad2 : Monitor} size={17} /></span>
          <span class="near-text">
            <span class="near-name">{d.deviceName}</span>
            {#if sent}
              <span class="cap sent">{sent.code ? $t('devices.add.checkOn', { name: d.deviceName }) : $t('devices.add.acceptOn', { name: d.deviceName })}</span>
            {:else}
              <span class="cap">{d.address}</span>
            {/if}
          </span>
          {#if sent?.code}
            <CheckDigits code={sent.code} />
          {:else if !sent}
            <button class="btn small primary" disabled={busy} on:click={() => pairNearby(d)}>{$t('devices.pair')}</button>
          {/if}
        </div>
      {:else}
        <p class="cap">{$t('devices.page.nearbyNone')}</p>
      {/each}
      {#if nearby.length > 4}<button class="btn small ghost" on:click={() => (adding = 'nearby')}>{$t('devices.page.seeAll', { n: nearby.length })}</button>{/if}
      <p class="cap">{$t('devices.page.nearbyHint')}</p>
    </section>
    <InternetSyncCard on:add={(e) => (adding = e.detail.way)} />
  </aside>
</div>

{#if adding}
  <AddDeviceDialog way={adding} on:close={() => (adding = '')} />
{/if}

<style>
  .head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    flex-wrap: wrap;
    margin-bottom: 20px;
  }
  .page-title {
    font-size: var(--text-2xl);
    font-weight: 600;
    letter-spacing: -0.01em;
  }
  .sub {
    margin-top: 2px;
    font-size: var(--text-md);
    color: var(--text-dim);
  }
  .btn.big {
    height: 40px;
    padding: 0 18px;
    font-size: var(--text-base);
  }
  .layout {
    display: flex;
    gap: 20px;
    align-items: flex-start;
    flex-wrap: wrap;
  }
  .main-col {
    flex: 999 1 560px;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 14px;
  }
  .side {
    flex: 1 1 300px;
    max-width: 360px;
    display: flex;
    flex-direction: column;
    gap: 14px;
  }
  /* Below 900px the side column moves under the devices (scaling rules). */
  @media (max-width: 900px) {
    .side {
      max-width: none;
    }
  }
  .here {
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 12px 16px;
    flex-wrap: wrap;
  }
  .tile {
    width: 44px;
    height: 44px;
    border-radius: 12px;
    display: grid;
    place-items: center;
    flex-shrink: 0;
    background: var(--accent-soft);
    color: var(--accent-text);
  }
  .here-who {
    flex: 1 1 200px;
    min-width: 0;
  }
  .here-name {
    font-weight: 600;
    font-size: var(--text-lg);
  }
  .cap {
    font-size: var(--text-sm);
    color: var(--text-faint);
    font-weight: 400;
  }
  .here-facts {
    display: flex;
    gap: 24px;
    margin: 0;
    flex-wrap: wrap;
  }
  dt {
    font-size: var(--text-xs);
    color: var(--text-faint);
  }
  dd {
    margin: 0;
    font-weight: 600;
    font-size: var(--text-md);
  }
  .mono {
    font-family: 'Cascadia Mono', Consolas, monospace;
  }
  .rename {
    display: flex;
    gap: 8px;
    align-items: center;
  }
  .rename input {
    flex: 1;
    min-width: 0;
    height: 32px;
    padding: 0 10px;
    border: 1px solid var(--accent);
    border-radius: 8px;
    background: var(--bg);
    color: var(--text);
    font: inherit;
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
    gap: 14px;
  }
  .cell {
    display: grid;
    min-width: 0;
  }
  .empty-paired {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 8px;
    padding: 24px;
  }
  .empty-paired p {
    color: var(--text-dim);
    margin-bottom: 6px;
  }
  .nearby {
    display: flex;
    flex-direction: column;
    gap: 10px;
    padding: 16px;
  }
  .side-head {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .side-head :global(svg) {
    color: var(--accent-text);
  }
  .side-head h2 {
    font-size: var(--text-base);
    font-weight: 600;
  }
  .near {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .near-tile {
    width: 34px;
    height: 34px;
    flex-shrink: 0;
    border-radius: 9px;
    display: grid;
    place-items: center;
    background: var(--bg-active);
    color: var(--text-dim);
  }
  .near-text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }
  .near-name {
    font-weight: 600;
    font-size: var(--text-md);
  }
  .cap.sent {
    color: var(--accent-text);
  }
</style>
