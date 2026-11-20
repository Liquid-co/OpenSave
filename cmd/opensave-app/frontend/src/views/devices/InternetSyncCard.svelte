<script>
  // Internet sync, on the Devices page: whether this device is in a room
  // and who else is, and which relay carries it — OpenSave's, another one,
  // or one this PC hosts.
  //
  // Hosting serves the devices at home or on Tailscale and nothing further:
  // the relay hosted here speaks ws://, and OpenSave accepts ws:// only on
  // this machine, a home network or Tailscale (store.ValidateRelayURL), so a
  // device elsewhere cannot use it. It says so rather than asking for a port
  // to be opened on the router to no end.
  import { createEventDispatcher, onMount } from 'svelte';
  import Globe from 'lucide-svelte/icons/globe';
  import Copy from 'lucide-svelte/icons/copy';
  import RadioTower from 'lucide-svelte/icons/radio-tower';
  import CircleCheck from 'lucide-svelte/icons/circle-check';
  import CircleX from 'lucide-svelte/icons/circle-x';
  import { settings, wanRoom, toast, askConfirm } from '../../lib/stores.js';
  import { api } from '../../lib/api.js';
  import { homeRelayAddress } from '../../lib/devicesview.js';
  import { collapse } from '../../lib/motion.js';
  import { t } from '../../lib/i18n.js';

  const dispatch = createEventDispatcher();
  const DEFAULT_RELAY = 'wss://relay.opensave.org';

  let busy = false;
  let ips = null;
  async function loadIps() {
    try {
      ips = await api.get('/api/relay/ips');
    } catch {
      ips = null;
    }
  }
  onMount(loadIps);

  $: inRoom = !!$wanRoom?.enabled;
  $: state = !inRoom ? 'off' : $wanRoom.connected ? 'on' : 'connecting';
  $: locked = !!$settings?.relayUrlLocked;
  $: choice = $settings?.hostRelay ? 'host' : ($settings?.relayUrl || DEFAULT_RELAY) === DEFAULT_RELAY ? 'opensave' : 'other';
  let picked = null;
  $: shown = picked ?? choice;
  let draftUrl = '';
  $: if (choice === 'other' && !draftUrl) draftUrl = $settings?.relayUrl ?? '';
  let port = 0;
  $: if (!port && $settings) port = $settings.relayPort || 8386;
  $: hosting = ips?.hosting ?? { running: !!$settings?.hostRelay, port: $settings?.relayPort };
  $: homeAddress = homeRelayAddress(ips?.lanIps, hosting.port || port);
  $: roomNames = [$settings?.deviceName, ...($wanRoom?.peers ?? []).map((p) => p.deviceName ?? p.name)].filter(Boolean);
  let health = null;

  async function save(patch, ok) {
    if (busy) return;
    busy = true;
    try {
      settings.set(await api.post('/api/settings', patch));
      if (inRoom) await api.post('/api/relay/reconnect').catch(() => {});
      picked = null;
      if (ok) toast(ok, 'success');
      setTimeout(loadIps, 1500);
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      busy = false;
    }
  }
  const useOpenSave = () => save({ hostRelay: false, relayUrl: DEFAULT_RELAY }, $t('devices.inet.usingOpenSave'));
  const useOther = () => save({ hostRelay: false, relayUrl: draftUrl.trim() }, $t('devices.inet.usingOther'));
  const startHosting = () =>
    save({ hostRelay: true, relayPort: Number(port), relayUrl: `ws://127.0.0.1:${Number(port)}` }, $t('devices.inet.hostingStarted', { port }));
  const stopHosting = () => save({ hostRelay: false, relayUrl: DEFAULT_RELAY }, $t('devices.inet.hostingStopped'));

  async function test() {
    health = null;
    try {
      health = await api.get('/api/relay/health');
    } catch (e) {
      health = { reachable: false, error: e.message };
    }
  }
  async function leave() {
    if (!(await askConfirm($t('devices.inet.leaveBody'), { title: $t('devices.inet.leaveTitle'), confirmText: $t('internetSync.leaveRoom') }))) return;
    save({ syncCode: '' }, $t('internetSync.leftRoomToast'));
  }
  function copy(text, ok) {
    navigator.clipboard?.writeText(text);
    toast(ok);
  }
</script>

<section class="card inet" aria-labelledby="inet-title">
  <div class="head">
    <Globe size={17} />
    <h2 id="inet-title">{$t('devices.inet.title')}</h2>
    <span class="pill state-{state}">{$t('devices.inet.state.' + state)}</span>
  </div>
  <p class="hint">{$t('devices.inet.explain')}</p>

  {#if inRoom}
    <div class="room">
      <span class="lbl">{$t('devices.inet.roomCode')}</span>
      <span class="code">{$wanRoom.roomCode}</span>
      <button class="icon-btn" title={$t('common.copy')} aria-label={$t('common.copy')} on:click={() => copy($wanRoom.roomCode, $t('internetSync.codeCopiedToast'))}><Copy size={14} /></button>
      <span class="lbl">{$t('devices.inet.inItNow')}</span>
      <span class="names">{roomNames.join(', ')}</span>
    </div>
    <div class="row-actions">
      <button class="btn small" on:click={() => dispatch('add', { way: 'code' })}>{$t('devices.inet.pairSomeone')}</button>
      <button class="btn small ghost" disabled={busy} on:click={leave}>{$t('internetSync.leaveRoom')}</button>
    </div>
  {:else}
    <button class="btn small" on:click={() => dispatch('add', { way: 'code' })}>{$t('devices.inet.join')}</button>
  {/if}

  <div class="relay" role="radiogroup" aria-labelledby="relay-label">
    <span id="relay-label" class="section">{$t('devices.inet.whichRelay')}</span>
    {#if locked}
      <p class="hint">{$t('internetSync.relayLockedBefore')} <code>OPENSAVE_RELAY_URL</code> {$t('internetSync.relayLockedAfter')}</p>
    {/if}
    {#each ['opensave', 'other', 'host'] as id}
      <button class="opt" class:on={shown === id} role="radio" aria-checked={shown === id} disabled={locked || busy} on:click={() => (picked = id)}>
        <span class="radio"></span>
        <span class="opt-text"><span class="opt-name">{$t('devices.inet.relay.' + id)}</span><span class="opt-sub">{$t('devices.inet.relay.' + id + 'Sub')}</span></span>
      </button>
    {/each}
  </div>

  {#if shown === 'opensave' && choice !== 'opensave'}
    <div class="panel" transition:collapse>
      <button class="btn small primary" disabled={busy} on:click={useOpenSave}>{$t('devices.inet.useThis')}</button>
    </div>
  {:else if shown === 'other'}
    <div class="panel" transition:collapse>
      <input class="url" placeholder="wss://relay.example.com" bind:value={draftUrl} aria-label={$t('internetSync.relayUrlLabel')} />
      <div class="row-actions">
        <button class="btn small primary" disabled={busy || !draftUrl.trim() || (choice === 'other' && draftUrl.trim() === $settings?.relayUrl)} on:click={useOther}>{$t('devices.inet.useThis')}</button>
        <button class="btn small" on:click={test}>{$t('internetSync.testRelay')}</button>
      </div>
      {#if health}
        <p class="health" class:ok={health.reachable}>
          {#if health.reachable}<CircleCheck size={14} />{$t('devices.inet.reachable')}{:else}<CircleX size={14} />{$t('devices.inet.unreachable')}{/if}
        </p>
      {/if}
    </div>
  {:else if shown === 'host'}
    <div class="panel host" transition:collapse>
      {#if choice === 'host'}
        <p class="status"><RadioTower size={15} />{hosting.running ? $t('devices.inet.hostingOn', { port: hosting.port || port }) : $t('devices.inet.hostingStarting', { port })}</p>
        {#if homeAddress}
          <span class="lbl">{$t('devices.inet.homeAddress')}</span>
          <span class="addr"><span class="mono">{homeAddress}</span><button class="icon-btn" title={$t('common.copy')} aria-label={$t('common.copy')} on:click={() => copy(homeAddress, $t('devices.inet.addressCopied'))}><Copy size={14} /></button></span>
        {/if}
        <p class="hint">{$t('devices.inet.hostLimits')}</p>
        <button class="btn small ghost" disabled={busy} on:click={stopHosting}>{$t('devices.inet.stopHosting')}</button>
      {:else}
        <p class="hint">{$t('devices.inet.hostLimits')}</p>
        <label class="port">{$t('settings.sync.relayPort')}<input type="number" min="1024" max="65535" bind:value={port} /></label>
        <button class="btn small primary" disabled={busy || !port} on:click={startHosting}>{$t('devices.inet.startHosting')}</button>
      {/if}
    </div>
  {/if}
</section>

<style>
  .inet {
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 16px;
  }
  .head {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .head :global(svg) {
    color: var(--accent-text);
  }
  h2 {
    flex: 1;
    font-size: var(--text-base);
    font-weight: 600;
  }
  .pill {
    font-size: var(--text-xs);
    font-weight: 600;
    padding: 2px 9px;
    border-radius: 999px;
    background: var(--bg-active);
    color: var(--text-dim);
  }
  .state-on {
    background: rgba(var(--success-rgb), 0.14);
    color: var(--success);
  }
  .state-connecting {
    background: rgba(var(--warn-rgb), 0.14);
    color: var(--warn);
  }
  .room {
    display: grid;
    grid-template-columns: auto 1fr auto;
    align-items: center;
    gap: 6px 12px;
    font-size: var(--text-md);
  }
  .lbl {
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
  .code,
  .mono {
    font-family: 'Cascadia Mono', Consolas, monospace;
    font-size: var(--text-md);
    font-weight: 600;
  }
  .names {
    grid-column: span 2;
    font-weight: 600;
  }
  .icon-btn {
    width: 28px;
    height: 28px;
    display: grid;
    place-items: center;
    border: none;
    border-radius: 7px;
    background: transparent;
    color: var(--text-dim);
    cursor: pointer;
  }
  .icon-btn:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .row-actions {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
  }
  .relay {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding-top: 12px;
    border-top: 1px solid var(--border);
  }
  .section {
    font-size: var(--text-sm);
    font-weight: 600;
    color: var(--text-faint);
    margin-bottom: 4px;
  }
  .opt {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 8px 10px;
    border: 1px solid transparent;
    border-radius: 10px;
    background: transparent;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
    transition:
      background 0.12s,
      border-color 0.12s;
  }
  .opt:hover:not(:disabled) {
    background: var(--bg-hover);
  }
  .opt.on {
    border-color: rgba(var(--accent-rgb), 0.5);
    background: var(--accent-soft);
  }
  .opt:disabled {
    opacity: 0.55;
    cursor: default;
  }
  .radio {
    width: 16px;
    height: 16px;
    flex-shrink: 0;
    border-radius: 50%;
    border: 2px solid var(--border-strong);
    transition: border-width 0.15s;
  }
  .opt.on .radio {
    border: 5px solid var(--accent);
  }
  .opt-text {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .opt-name {
    font-weight: 600;
    font-size: var(--text-md);
  }
  .opt-sub {
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
  .panel {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 8px;
    padding: 12px;
    border-radius: 12px;
    background: var(--bg-hover);
    border: 1px solid var(--border-strong);
  }
  .url,
  .port input {
    width: 100%;
    height: 34px;
    padding: 0 10px;
    border: 1px solid var(--border-strong);
    border-radius: 9px;
    background: var(--bg);
    color: var(--text);
    font: inherit;
    font-size: var(--text-md);
  }
  .port {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
  .port input {
    width: 110px;
  }
  .status {
    display: flex;
    align-items: center;
    gap: 8px;
    font-weight: 600;
    font-size: var(--text-md);
  }
  .status :global(svg) {
    color: var(--success);
  }
  .addr {
    display: flex;
    align-items: center;
    gap: 6px;
    width: 100%;
    padding: 2px 4px 2px 10px;
    border-radius: 9px;
    background: var(--bg);
    border: 1px solid var(--border-strong);
  }
  .addr .mono {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .health {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: var(--text-md);
    color: var(--danger-text);
  }
  .health.ok {
    color: var(--success);
  }
</style>
