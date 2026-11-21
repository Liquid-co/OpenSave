<script>
  // Add a device: three ways for two devices to find each other. Nearby, on
  // the same network, drawn on a radar (lib/radar.js) beside the list; with
  // a code, over the internet, both joining the same 12-character room
  // (lib/roomcode.js); or by typing the other device's address.
  //
  // Each way ends the same: this device asks, and the other device accepts.
  import { createEventDispatcher } from 'svelte';
  import Wifi from 'lucide-svelte/icons/wifi';
  import Globe from 'lucide-svelte/icons/globe';
  import Hash from 'lucide-svelte/icons/hash';
  import Gamepad2 from 'lucide-svelte/icons/gamepad-2';
  import Monitor from 'lucide-svelte/icons/monitor';
  import Dices from 'lucide-svelte/icons/dices';
  import Copy from 'lucide-svelte/icons/copy';
  import Search from 'lucide-svelte/icons/search';
  import Clock from 'lucide-svelte/icons/clock';
  import Check from 'lucide-svelte/icons/check';
  import Info from 'lucide-svelte/icons/info';
  import MonitorSmartphone from 'lucide-svelte/icons/monitor-smartphone';
  import Modal from './ui/Modal.svelte';
  import CheckDigits from './ui/CheckDigits.svelte';
  import { peers, discoveredPeers, wanRoom, settings, toast } from '../lib/stores.js';
  import { api } from '../lib/api.js';
  import { isHandheld } from '../lib/devices.js';
  import { generateRoomCode } from '../lib/roomcode.js';
  import { placeOnRadar, LABELLED_UP_TO } from '../lib/radar.js';
  import { rise, pop } from '../lib/motion.js';
  import { sentPairings, pairClock, askToPair, waitingOn } from '../lib/pairing.js';
  import { t } from '../lib/i18n.js';

  /** 'nearby', 'code' or 'address': which way to open on. */
  export let way = 'nearby';
  const dispatch = createEventDispatcher();
  const close = () => dispatch('close');

  let busy = false;
  // Asked, and waiting on the other device to accept (lib/pairing.js).
  $: wait = (key) => waitingOn($sentPairings, key, $pairClock);

  async function run(fn) {
    if (busy) return false;
    busy = true;
    try {
      await fn();
      return true;
    } catch (e) {
      toast(e.message, 'error');
      return false;
    } finally {
      busy = false;
    }
  }

  // ── Nearby ──
  $: pairedIds = new Set(Object.keys($peers));
  $: nearby = $discoveredPeers.filter((d) => !d.isWan && !pairedIds.has(d.id));
  $: radar = placeOnRadar(nearby.map((d) => ({ ...d, name: d.deviceName })), { width: 340, height: 320 });
  let query = '';
  let kind = 'all';
  const kindOf = (d) => (isHandheld(d.deviceType) ? 'handheld' : 'computer');
  $: listed = nearby
    .filter((d) => kind === 'all' || kindOf(d) === kind)
    .filter((d) => !query.trim() || `${d.deviceName} ${d.address}`.toLowerCase().includes(query.trim().toLowerCase()));
  const pairNearby = (d) => run(() => askToPair(d.id, d.deviceName, { address: d.address, port: d.port }));

  // ── With a code ──
  $: inRoom = !!$wanRoom?.enabled;
  let fresh = generateRoomCode();
  $: code = inRoom ? $wanRoom.roomCode : fresh;
  $: groups = String(code ?? '').split('-');
  let theirs = '';
  async function joinRoom(c) {
    const ok = await run(async () => {
      settings.set(await api.post('/api/settings', { syncCode: c.trim() }));
    });
    if (ok) toast($t('internetSync.joiningRoomToast', { code: c.trim() }));
  }
  $: roomPeers = ($wanRoom?.peers ?? []).filter((p) => p.id);
  const pairRoom = (p) => run(() => askToPair(p.id, p.deviceName, { peerId: p.id, address: 'relay' }));

  // ── By address ──
  let ip = '';
  let port = 8383;
  $: typed = `${ip.trim()}:${port}`;
  const pairAddress = () => run(() => askToPair(typed, typed, { address: ip.trim(), port: Number(port) }));
  $: typedWait = wait(typed);

  const WAYS = [
    { id: 'nearby', icon: Wifi },
    { id: 'code', icon: Globe },
    { id: 'address', icon: Hash }
  ];
</script>

<Modal title={$t('devices.add.title')} icon={MonitorSmartphone} width={920} height="auto" onClose={close}>
  <svelte:fragment slot="sub">{$t('devices.add.sub')}</svelte:fragment>

  <div class="ways" role="radiogroup" aria-label={$t('devices.add.how')}>
    {#each WAYS as w}
      <button class="way" class:on={way === w.id} role="radio" aria-checked={way === w.id} on:click={() => (way = w.id)}>
        <span class="wt"><svelte:component this={w.icon} size={21} /></span>
        <span><span class="wname">{$t('devices.add.way.' + w.id)}</span><span class="wsub">{$t('devices.add.way.' + w.id + 'Sub')}</span></span>
      </button>
    {/each}
  </div>

  <div class="body">
    {#if way === 'nearby'}
      <div class="split" in:rise={{ y: 6 }}>
        <div class="radar-col">
          <div class="radar" aria-hidden="true">
            <span class="ring r1"></span><span class="ring r2"></span><span class="ring r3"></span>
            <span class="sweep"></span>
            <span class="me"><Monitor size={22} /></span>
            {#each radar.placed as p (p.device.id)}
              <span class="pip" class:asked={wait(p.device.id)} style="left: {p.x}px; top: {p.y}px" in:pop={{ start: 0.4 }}>
                <svelte:component this={isHandheld(p.device.deviceType) ? Gamepad2 : Monitor} size={15} />
              </span>
              {#if radar.labelled}<span class="lbl" style="left: {p.x}px; top: {p.y + 20}px">{p.device.deviceName}</span>{/if}
            {/each}
            {#if radar.hidden > 0}<span class="more-pips">+{radar.hidden}</span>{/if}
          </div>
          <p class="cap">{radar.hidden > 0 ? $t('devices.add.radarMore', { n: radar.placed.length, total: nearby.length }) : $t('devices.add.looking')}</p>
        </div>
        <div class="list-col">
          <div class="list-head"><h3>{nearby.length ? $t('devices.add.found', { n: nearby.length }) : $t('devices.add.noneYet')}</h3><span class="cap">{$t('devices.add.stillLooking')}</span></div>
          {#if nearby.length > LABELLED_UP_TO}
            <label class="find"><Search size={14} /><input placeholder={$t('devices.add.search')} bind:value={query} /></label>
            <div class="chips">
              {#each ['all', 'handheld', 'computer'] as k}
                <button class="chip" class:on={kind === k} on:click={() => (kind = k)}>{$t('devices.add.kind.' + k)}</button>
              {/each}
            </div>
          {/if}
          <div class="rows">
            {#each listed as d (d.id)}
              {@const sent = wait(d.id)}
              <div class="row" class:waiting={sent} in:rise={{ y: 4 }}>
                <span class="dtile"><svelte:component this={isHandheld(d.deviceType) ? Gamepad2 : Monitor} size={18} /></span>
                <span class="dtext">
                  <span class="dname">{d.deviceName}</span>
                  {#if sent?.code}
                    <span class="dsub check">{$t('devices.add.checkOn', { name: d.deviceName })}</span>
                  {:else}
                    <span class="dsub">{d.address}</span>
                  {/if}
                </span>
                {#if sent?.code}
                  <CheckDigits code={sent.code} />
                {:else if sent}
                  <span class="wait"><Clock size={14} />{$t('devices.add.acceptOn', { name: d.deviceName })}</span>
                {:else}
                  <button class="btn small primary" disabled={busy} on:click={() => pairNearby(d)}>{$t('devices.pair')}</button>
                {/if}
              </div>
            {:else}
              <div class="empty-rows">
                <span class="dot-pulse"></span>
                <p>{$t('devices.add.openOnOther')}</p>
              </div>
            {/each}
          </div>
        </div>
      </div>
    {:else if way === 'code'}
      <div class="split" in:rise={{ y: 6 }}>
        <section class="code-card">
          <h3>{inRoom ? $t('devices.add.codeInRoom') : $t('devices.add.codeNew')}</h3>
          <p class="cap">{$t('devices.add.codeHow')}</p>
          <div class="code" aria-label={code}>
            {#each groups as g, i}
              <span class="g4">{#each g.split('') as c}<span>{c}</span>{/each}</span>
              {#if i < groups.length - 1}<span class="dash">-</span>{/if}
            {/each}
          </div>
          <div class="row-actions">
            <button class="btn small" on:click={() => { navigator.clipboard?.writeText(code); toast($t('internetSync.codeCopiedToast')); }}><Copy size={14} />{$t('common.copy')}</button>
            {#if !inRoom}
              <button class="btn small ghost" on:click={() => (fresh = generateRoomCode())}><Dices size={14} />{$t('devices.add.newCode')}</button>
              <button class="btn small primary" disabled={busy} on:click={() => joinRoom(code)}>{$t('devices.add.useCode')}</button>
            {/if}
          </div>
          <div class="or"><span></span>{$t('devices.add.or')}<span></span></div>
          <label class="their">
            <span class="cap">{$t('devices.add.gotCode')}</span>
            <span class="their-row"><input placeholder="xxxx-xxxx-xxxx" bind:value={theirs} /><button class="btn small" disabled={busy || theirs.trim().length < 6} on:click={() => joinRoom(theirs)}>{$t('internetSync.joinRoom')}</button></span>
          </label>
          <p class="note">{$t('devices.add.codeNote')}</p>
        </section>
        <section class="room-col">
          <div class="list-head"><h3>{$t('devices.add.inRoom')}</h3>{#if inRoom}<span class="cap">{$wanRoom.connected ? $t('devices.add.throughRelay') : $t('devices.inet.state.connecting')}</span>{/if}</div>
          <div class="rows">
            {#each roomPeers as p (p.id)}
              <div class="row" in:rise={{ y: 4 }}>
                <span class="dtile"><svelte:component this={isHandheld(p.deviceType) ? Gamepad2 : Monitor} size={18} /></span>
                <span class="dtext"><span class="dname">{p.deviceName}</span></span>
                {#if pairedIds.has(p.id)}
                  <span class="paired"><Check size={13} />{$t('devices.add.paired')}</span>
                {:else if wait(p.id)}
                  <span class="wait"><Clock size={14} />{$t('devices.add.acceptOn', { name: p.deviceName })}</span>
                {:else}
                  <button class="btn small primary" disabled={busy} on:click={() => pairRoom(p)}>{$t('devices.pair')}</button>
                {/if}
              </div>
            {:else}
              <div class="empty-rows"><p>{inRoom ? $t('devices.add.roomEmpty') : $t('devices.add.joinFirst')}</p></div>
            {/each}
          </div>
        </section>
      </div>
    {:else}
      <div class="address" in:rise={{ y: 6 }}>
        <p class="cap">{$t('devices.add.addressHow')}</p>
        <div class="addr-row">
          <label class="field-l"><span>{$t('devices.add.ip')}</span><input placeholder="192.168.1.42" bind:value={ip} /></label>
          <label class="field-l port"><span>{$t('devices.add.port')}</span><input type="number" bind:value={port} /></label>
          <button class="btn primary" disabled={busy || !ip.trim()} on:click={pairAddress}>{$t('devices.sendPairing')}</button>
        </div>
        {#if typedWait?.code}
          <div class="typed-check" in:rise={{ y: 4 }}>
            <CheckDigits code={typedWait.code} size="md" />
            <p>{$t('devices.add.checkThere')}</p>
          </div>
        {:else if typedWait}
          <p class="wait big"><Clock size={15} />{$t('devices.add.acceptThere')}</p>
        {/if}
      </div>
    {/if}
  </div>

  <div class="foot">
    <Info size={15} />
    <span>{way === 'nearby' ? $t('devices.add.footNearby') : way === 'code' ? $t('devices.add.footCode') : $t('devices.add.footAddress')}</span>
    <button class="btn" on:click={close}>{$t('devices.add.done')}</button>
  </div>
</Modal>

<style>
  .ways {
    flex-shrink: 0;
    display: flex;
    gap: 10px;
    padding: 18px 22px 0;
  }
  .way {
    flex: 1;
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 12px 14px;
    border-radius: 14px;
    border: 1.5px solid var(--border);
    background: var(--bg);
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
    transition:
      border-color 0.15s,
      background 0.15s;
  }
  .way:hover {
    border-color: var(--border-strong);
  }
  .way.on {
    border-color: var(--accent);
    background: linear-gradient(0deg, var(--accent-soft), var(--accent-soft)), var(--bg);
  }
  .wt {
    width: 42px;
    height: 42px;
    flex-shrink: 0;
    border-radius: 12px;
    display: grid;
    place-items: center;
    background: var(--bg-active);
    color: var(--text-dim);
    transition: color 0.15s;
  }
  .way.on .wt {
    background: rgba(var(--accent-rgb), 0.22);
    color: var(--accent-text);
  }
  .wname {
    display: block;
    font-weight: 600;
  }
  .wsub {
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
  .body {
    padding: 18px 22px 4px;
    min-height: 360px;
  }
  .split {
    display: flex;
    gap: 20px;
    align-items: stretch;
  }
  .radar-col {
    flex: none;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .radar {
    position: relative;
    width: 340px;
    height: 320px;
    border-radius: 16px;
    overflow: hidden;
    background: radial-gradient(circle at 50% 50%, rgba(var(--accent-rgb), 0.12), transparent 70%), var(--bg);
    border: 1px solid var(--border);
  }
  .ring {
    position: absolute;
    left: 50%;
    top: 50%;
    border-radius: 50%;
    border: 1px solid rgba(var(--accent-rgb), 0.24);
    transform: translate(-50%, -50%);
  }
  .r1 {
    width: 120px;
    height: 120px;
  }
  .r2 {
    width: 220px;
    height: 220px;
  }
  .r3 {
    width: 300px;
    height: 300px;
    border-style: dashed;
  }
  /* The sweep: a wedge of accent turning once every few seconds while it
     looks. Stilled with animations off. */
  .sweep {
    position: absolute;
    left: 50%;
    top: 50%;
    width: 300px;
    height: 300px;
    margin: -150px 0 0 -150px;
    border-radius: 50%;
    background: conic-gradient(from 0deg, rgba(var(--accent-rgb), 0.22), transparent 60deg);
    animation: sweep 4s linear infinite;
  }
  @keyframes sweep {
    to {
      transform: rotate(360deg);
    }
  }
  .me {
    position: absolute;
    left: 50%;
    top: 50%;
    width: 52px;
    height: 52px;
    transform: translate(-50%, -50%);
    border-radius: 15px;
    display: grid;
    place-items: center;
    background: var(--accent-fill);
    color: var(--on-accent);
    box-shadow: 0 0 0 6px rgba(var(--accent-rgb), 0.22);
  }
  .pip {
    position: absolute;
    width: 34px;
    height: 34px;
    margin: -17px 0 0 -17px;
    border-radius: 50%;
    display: grid;
    place-items: center;
    background: var(--bg-raised);
    border: 1px solid var(--border-strong);
    color: var(--success);
  }
  .pip.asked {
    border-color: var(--accent);
    box-shadow: 0 0 0 4px rgba(var(--accent-rgb), 0.28);
  }
  .lbl {
    position: absolute;
    transform: translateX(-50%);
    max-width: 112px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--text-sm);
    font-weight: 600;
  }
  .more-pips {
    position: absolute;
    right: 10px;
    bottom: 10px;
    padding: 2px 10px;
    border-radius: 999px;
    background: var(--bg-active);
    border: 1px solid var(--border-strong);
    font-size: var(--text-sm);
    font-weight: 600;
  }
  .cap {
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
  .radar-col .cap {
    text-align: center;
  }
  .list-col,
  .room-col {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .list-head {
    display: flex;
    align-items: baseline;
    gap: 8px;
  }
  h3 {
    font-size: var(--text-base);
    font-weight: 600;
  }
  .find {
    display: flex;
    align-items: center;
    gap: 8px;
    height: 34px;
    padding: 0 10px;
    border: 1px solid var(--border-strong);
    border-radius: 9px;
    background: var(--bg);
    color: var(--text-faint);
  }
  .find input {
    flex: 1;
    min-width: 0;
    border: none;
    outline: none;
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: var(--text-md);
  }
  .chips {
    display: flex;
    gap: 6px;
  }
  .chip {
    height: 28px;
    padding: 0 11px;
    border-radius: 999px;
    border: 1px solid var(--border);
    background: transparent;
    color: var(--text-dim);
    font: inherit;
    font-size: var(--text-sm);
    cursor: pointer;
  }
  .chip.on {
    background: var(--bg-active);
    border-color: var(--border-strong);
    color: var(--text);
    font-weight: 600;
  }
  .rows {
    display: flex;
    flex-direction: column;
    gap: 6px;
    max-height: 300px;
    overflow-y: auto;
  }
  .row {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 9px 12px;
    border-radius: 12px;
    background: var(--bg);
    border: 1px solid var(--border);
    transition: border-color 0.2s;
  }
  .row.waiting {
    border-color: rgba(var(--accent-rgb), 0.5);
  }
  .dtile {
    width: 36px;
    height: 36px;
    flex-shrink: 0;
    border-radius: 10px;
    display: grid;
    place-items: center;
    background: var(--bg-active);
    color: var(--success);
  }
  .dtext {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }
  .dname {
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .dsub {
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
  .wait,
  .paired {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: var(--text-sm);
    color: var(--accent-text);
    white-space: nowrap;
  }
  .paired {
    color: var(--success);
  }
  .wait.big {
    margin-top: 12px;
    font-size: var(--text-md);
  }
  .dsub.check {
    color: var(--accent-text);
  }
  .typed-check {
    display: flex;
    align-items: center;
    gap: 14px;
    margin-top: 12px;
    padding: 12px 14px;
    border-radius: 12px;
    border: 1px solid rgba(var(--accent-rgb), 0.5);
    background: var(--bg);
  }
  .typed-check p {
    color: var(--text-dim);
    font-size: var(--text-md);
    line-height: 1.45;
  }
  .empty-rows {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 10px;
    min-height: 140px;
    padding: 16px;
    border: 1.5px dashed var(--border-strong);
    border-radius: 12px;
    text-align: center;
    color: var(--text-dim);
    font-size: var(--text-md);
  }
  .dot-pulse {
    width: 10px;
    height: 10px;
    border-radius: 50%;
    background: var(--accent);
    animation: pulse 1.6s ease-in-out infinite;
  }
  @keyframes pulse {
    50% {
      transform: scale(1.6);
      opacity: 0.4;
    }
  }
  .code-card {
    flex: 1.1 1 0;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 16px;
    border-radius: 16px;
    background: var(--bg);
    border: 1px solid var(--border);
  }
  .code {
    container-type: inline-size;
    display: grid;
    grid-template-columns: 1fr auto 1fr auto 1fr;
    align-items: center;
    gap: 6px;
  }
  .g4 {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 3px;
    padding: 4px;
    border-radius: 10px;
    background: var(--bg-raised);
    border: 1px solid var(--border-strong);
    min-width: 0;
  }
  .g4 span {
    aspect-ratio: 5 / 7;
    border-radius: 6px;
    display: grid;
    place-items: center;
    background: var(--bg-active);
    font-family: 'Cascadia Mono', Consolas, monospace;
    font-size: clamp(13px, 4.6cqw, 19px);
    font-weight: 600;
    min-width: 0;
  }
  .dash {
    color: var(--text-faint);
    font-weight: 700;
  }
  .row-actions {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
  }
  .or {
    display: flex;
    align-items: center;
    gap: 10px;
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
  .or span {
    flex: 1;
    height: 1px;
    background: var(--border);
  }
  .their {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .their-row {
    display: flex;
    gap: 8px;
  }
  .their input,
  .addr-row input {
    flex: 1;
    min-width: 0;
    height: 36px;
    padding: 0 12px;
    border: 1px solid var(--border-strong);
    border-radius: 9px;
    background: var(--bg);
    color: var(--text);
    font: inherit;
  }
  .their input {
    font-family: 'Cascadia Mono', Consolas, monospace;
    letter-spacing: 0.04em;
  }
  .note {
    font-size: var(--text-sm);
    color: var(--text-faint);
    line-height: 1.45;
  }
  .address {
    display: flex;
    flex-direction: column;
    gap: 12px;
    max-width: 620px;
  }
  .addr-row {
    display: flex;
    align-items: flex-end;
    gap: 10px;
  }
  .field-l {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 6px;
    font-size: var(--text-md);
    font-weight: 600;
    color: var(--text-dim);
  }
  .field-l.port {
    flex: 0 0 110px;
  }
  .foot {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    gap: 10px;
    margin-top: 16px;
    padding: 14px 20px 14px 22px;
    border-top: 1px solid var(--border);
    background: var(--bg-sidebar);
    border-radius: 0 0 var(--modal-r, 14px) var(--modal-r, 14px);
    color: var(--text-faint);
    font-size: var(--text-md);
  }
  .foot span {
    flex: 1;
  }
</style>
