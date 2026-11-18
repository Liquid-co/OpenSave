<script>
  import { settings, wanRoom, peers, toast } from '../lib/stores.js';
  import { api } from '../lib/api.js';
  import { t } from '../lib/i18n.js';
  import { generateRoomCode } from '../lib/roomcode.js';
  import { roomProtection } from '../lib/protection.js';
  import DeviceIcon from '../components/DeviceIcon.svelte';
  import Lock from 'lucide-svelte/icons/lock';
  import LockOpen from 'lucide-svelte/icons/lock-open';
  import Dices from 'lucide-svelte/icons/dices';
  import CircleCheck from 'lucide-svelte/icons/circle-check';
  import CircleX from 'lucide-svelte/icons/circle-x';

  let codeDraft = '';
  let relayDraft = '';
  let busy = false;
  let health = null;

  // Seeded once, when the fields are still blank — which includes every time
  // this view is rebuilt after a tab switch, because the drafts are component
  // state and do not survive that.
  //
  // The room code comes from the live room in preference to the saved setting.
  // They normally agree, but the room is the thing actually joined, and it can
  // be changed from the CLI or another window; showing the stored code instead
  // would tell you that you are in a room you have already left.
  $: if ($settings && codeDraft === '' && relayDraft === '') {
    codeDraft = $wanRoom?.roomCode || ($settings.syncCode ?? '');
    relayDraft = $settings.relayUrl ?? '';
  }

  // Every save here has to put the server's answer back into the store.
  // Nothing else does: the store is filled by the `init` message at launch and
  // by the Settings view saving its own form, so a relay or room code changed
  // from this view stayed invisible to the rest of the app — and to this view
  // itself once a tab switch destroyed it and the fields above re-seeded from
  // the settings as they were before the change.
  async function saveSettings(patch) {
    settings.set(await api.post('/api/settings', patch));
  }
  $: pairedIds = new Set(Object.keys($peers));
  $: roomPeers = $wanRoom?.peers ?? [];

  // Which of the devices paired over this relay are actually protected.
  //
  // Answered here, where somebody joins a room and decides how much to trust
  // it, rather than left to a paragraph they have to map onto their own setup.
  // The condition mirrors what the send path applies, so this can never say
  // "encrypted" about traffic that is going out in the clear.
  $: ({
    relayPeers,
    encrypted: encryptedPeers,
    unprotected: unprotectedPeers,
  } = roomProtection(Object.values($peers)));

  function randomCode() {
    codeDraft = generateRoomCode();
  }

  async function run(fn, okMsg) {
    if (busy) return;
    busy = true;
    try {
      await fn();
      if (okMsg) toast(okMsg, 'success');
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      busy = false;
    }
  }

  // Each outcome gets its own toast — joining is announced as in-progress
  // (the status banner reports the eventual result), leaving as completed.
  const joinRoom = () =>
    run(async () => {
      const code = codeDraft.trim();
      if (!code) {
        if ($wanRoom?.enabled) {
          await saveSettings({ syncCode: '', relayUrl: relayDraft.trim() });
          toast($t('internetSync.leftRoomToast'), 'success');
        } else {
          toast($t('internetSync.enterCodeFirst'), 'error');
        }
        return;
      }
      const rejoining = code === ($wanRoom?.roomCode ?? '') && relayDraft.trim() === ($settings?.relayUrl ?? '');
      await saveSettings({ syncCode: code, relayUrl: relayDraft.trim() });
      // Saving identical settings doesn't re-dial, so force a fresh attempt.
      if (rejoining) await api.post('/api/relay/reconnect');
      toast(rejoining ? $t('internetSync.reconnectingRoomToast', { code }) : $t('internetSync.joiningRoomToast', { code }));
    });

  const retryConnect = () =>
    run(async () => {
      await api.post('/api/relay/reconnect');
      toast($t('internetSync.reconnectingRelayToast'));
    });
  const leaveRoom = () => {
    codeDraft = '';
    return run(() => saveSettings({ syncCode: '' }), $t('internetSync.leftRoomToast'));
  };
  const pairWan = (p) =>
    run(() => api.post('/api/peers/pair', { peerId: p.id, address: 'relay' }), $t('devices.pairingSentTo', { name: p.deviceName }));

  async function checkHealth() {
    health = null;
    try {
      health = await api.get('/api/relay/health');
    } catch (e) {
      toast(e.message, 'error');
    }
  }

  function copyCode() {
    navigator.clipboard?.writeText($wanRoom?.roomCode ?? codeDraft);
    toast($t('internetSync.codeCopiedToast'));
  }
</script>

<p class="lead">
  {$t('internetSync.lead')}
</p>

<!-- Said here, at the point where somebody decides whether to use the public
     relay, rather than only in the documentation.

     This text used to say the relay could read the saves passing through it,
     which was true and is not any more: save payloads are now sealed between
     the two paired devices with the keys pinned when they paired. What the
     relay still sees is metadata — that two devices are talking, roughly how
     much data, and which games by id — and that is what the second sentence
     is for. Overstating this would be worse than the old wording was: someone
     deciding whether to trust a relay deserves the actual boundary, not a
     reassuring version of it. -->
<p class="lead subtle">
  {$t('internetSync.sealedBefore')}
  <a href="https://github.com/Liquid-co/OpenSave/blob/main/docs/RELAY.md" target="_blank" rel="noreferrer">{$t('internetSync.ownRelayLink')}</a>
  {$t('internetSync.sealedAfter')}
</p>

<!-- Stated as a plain instruction rather than a condition the reader has to
     work out. Every pairing made over a relay before this version has no key:
     the key was sent during pairing and quietly discarded on the receiving
     side, so "a pairing made before keys existed" was the wrong caveat — it
     let someone on the previous version conclude this did not apply to them.
     It applied to all of them. -->
<p class="lead subtle">
  {$t('internetSync.repairNote')}
</p>

<!-- Always-visible connection status: exactly one of four states. Keyed
     off the live wanRoom broadcast (settings only arrive at WS init and
     would go stale after a join). -->
{#if !$wanRoom?.enabled}
  <div class="status idle">
    <span class="status-dot gray"></span>
    <div class="status-text">
      <strong>{$t('internetSync.statusIdleTitle')}</strong>
      <span>{$t('internetSync.statusIdleHintBefore')} <strong>{$t('internetSync.joinRoom')}</strong> {$t('internetSync.statusIdleHintAfter')}</span>
    </div>
  </div>
{:else if $wanRoom?.connected}
  <div class="status ok">
    <span class="status-dot green"></span>
    <div class="status-text">
      <strong>{$t('internetSync.statusInRoom', { code: $wanRoom.roomCode })}</strong>
      <span>{roomPeers.length === 0 ? $t('internetSync.waitingForOther') : $t('internetSync.othersHere', { n: roomPeers.length })}</span>
      <!-- Said at the moment of joining, because that is when it matters:
           every device in a room receives every other device's traffic, so
           whether that traffic is readable is part of what joining means. -->
      {#if relayPeers.length === 0}
        <!-- Nothing paired through this room yet, which is exactly when
             someone looks here wanting to know what joining means. Said as a
             forward-looking fact, and conditional on both devices being on a
             version that exchanges keys over a relay — earlier ones did not. -->
        <span class="encryption-line">
          <Lock size={14} class="inline-icon" />{$t('internetSync.encNone')}
        </span>
      {:else}
        <span class="encryption-line">
          {#if unprotectedPeers.length === 0}
            <Lock size={14} class="inline-icon" />{$t('internetSync.encAll', { target: relayPeers.length === 1 ? $t('internetSync.targetOne') : $t('internetSync.targetAll', { n: relayPeers.length }) })}
          {:else if encryptedPeers.length === 0}
            <LockOpen size={14} class="inline-icon warn-icon" />{$t('internetSync.encWarnBefore')} <strong>{$t('internetSync.notEncrypted')}</strong>{$t('internetSync.encWarnAfter', { target: unprotectedPeers.length === 1 ? $t('internetSync.targetNamed', { name: unprotectedPeers[0].name }) : $t('internetSync.targetThose') })}
          {:else}
            <LockOpen size={14} class="inline-icon warn-icon" />{$t('internetSync.encMixed', { done: encryptedPeers.length, total: relayPeers.length, target: unprotectedPeers.length === 1 ? $t('internetSync.targetOtherOne') : $t('internetSync.targetOthers'), pron: unprotectedPeers.length === 1 ? $t('internetSync.pronIt') : $t('internetSync.pronThem') })}
          {/if}
        </span>
      {/if}
    </div>
  </div>
{:else if $wanRoom?.state === 'connecting'}
  <div class="status wait">
    <span class="sspin"></span>
    <div class="status-text">
      <strong>{$t('internetSync.statusConnecting')}</strong>
      <span>{$t('internetSync.connectingHint')}</span>
    </div>
  </div>
{:else}
  <div class="status err">
    <span class="status-dot red"></span>
    <div class="status-text">
      <strong>{$t('internetSync.statusNotConnected')}</strong>
      <span>{$wanRoom?.error ?? $t('internetSync.relayDown')} {$t('internetSync.retryingEveryFew')}</span>
    </div>
    <button class="btn small" disabled={busy} on:click={retryConnect}>{$t('internetSync.retryNow')}</button>
  </div>
{/if}

<div class="card">
  <h3>{$t('internetSync.relayRoomTitle')}</h3>
  <div class="row">
    <div class="field grow">
      <label for="room-code">{$t('internetSync.roomCodeLabel')}</label>
      <div class="code-row">
        <input id="room-code" placeholder={$t('internetSync.codePlaceholder')} bind:value={codeDraft} />
        <button class="btn icon" on:click={randomCode} title={$t('internetSync.randomTitle')} aria-label={$t('internetSync.randomAria')}><Dices size={17} /></button>
        {#if $wanRoom?.enabled}
          <button class="btn" on:click={copyCode}>{$t('common.copy')}</button>
        {/if}
      </div>
    </div>
  </div>
  <div class="row">
    <div class="field grow">
      <label for="relay-url">{$t('internetSync.relayUrlLabel')}</label>
      <!-- Pinned by the environment: shown, because it is the relay actually
           in use, but not editable, because typing here would be discarded. -->
      <input id="relay-url" bind:value={relayDraft} readonly={$settings?.relayUrlLocked} />
      {#if $settings?.relayUrlLocked}
        <span class="hint">
          {$t('internetSync.relayLockedBefore')} <code>OPENSAVE_RELAY_URL</code> {$t('internetSync.relayLockedAfter')}
        </span>
      {:else}
        <span class="hint">{$t('internetSync.relaySelfHostHint')}</span>
      {/if}
    </div>
  </div>
  <div class="actions">
    <button class="btn" on:click={checkHealth}>{$t('internetSync.testRelay')}</button>
    {#if $wanRoom?.enabled}
      <button class="btn danger" disabled={busy} on:click={leaveRoom}>{$t('internetSync.leaveRoom')}</button>
    {/if}
    <button class="btn primary" disabled={busy} on:click={joinRoom}>
      {$wanRoom?.enabled ? $t('internetSync.update') : $t('internetSync.joinRoom')}
    </button>
  </div>
  {#if health}
    <div class="health with-icon" class:ok={health.reachable}>
      {#if health.reachable}
        <CircleCheck size={16} /> {$t('internetSync.relayOk', { devices: $t('internetSync.deviceCount', { n: health.health?.clients ?? 0 }), rooms: $t('internetSync.roomCount', { n: health.health?.rooms ?? 0 }) })}
      {:else}
        <CircleX size={16} /> {$t('internetSync.relayUnreachable', { error: health.error })}
      {/if}
    </div>
  {/if}
</div>

{#if $wanRoom?.enabled}
  <h3 class="section">{$t('internetSync.inThisRoom')}</h3>
  {#if roomPeers.length === 0}
    <p class="quiet">
      {$t('internetSync.noDevicesYet')}
    </p>
  {:else}
    <div class="list">
      {#each roomPeers as p (p.id)}
        <div class="card peer">
          <DeviceIcon type={p.deviceType} />
          <div class="peer-info">
            <div class="peer-name">
              {p.deviceName}
              <span class="badge" class:online={p.online} class:offline={!p.online}>{p.online ? $t('home.summary.online') : $t('internetSync.away')}</span>
            </div>
          </div>
          {#if p.paired || pairedIds.has(p.id)}
            <span class="badge online">{$t('internetSync.badgePaired')}</span>
          {:else}
            <button class="btn small primary" disabled={busy} on:click={() => pairWan(p)}>{$t('devices.pair')}</button>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
{/if}

<style>
  .lead {
    color: var(--text-dim);
    font-size: 0.9rem;
    max-width: 640px;
    margin-bottom: 20px;
  }
  /* The trust note under the lead. Quieter than the sentence above it, and
     deliberately not styled as a warning: nothing is wrong, it is a fact about
     the arrangement that the person choosing a relay should have. */
  .lead.subtle {
    color: var(--text-faint);
    font-size: 0.85rem;
    margin-top: -12px;
  }
  .lead.subtle a {
    color: var(--text-dim);
    text-decoration: underline;
  }
  .status {
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 14px 18px;
    border-radius: var(--radius-lg);
    border: 1px solid var(--border);
    background: var(--bg-raised);
    margin-bottom: 18px;
  }
  .status.ok {
    border-color: rgba(var(--success-rgb), 0.4);
    background: rgba(var(--success-rgb), 0.07);
  }
  .status.wait {
    border-color: rgba(var(--accent-rgb), 0.45);
    background: var(--accent-soft);
  }
  .status.err {
    border-color: rgba(var(--danger-rgb), 0.45);
    background: rgba(var(--danger-rgb), 0.08);
  }
  .encryption-line {
    display: block;
    margin-top: 0.3rem;
  }
  .status-text {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 2px;
    font-size: 0.9rem;
  }
  .status-text span {
    color: var(--text-dim);
    font-size: 0.82rem;
  }
  .status-dot {
    width: 10px;
    height: 10px;
    border-radius: 50%;
    flex-shrink: 0;
  }
  .status-dot.green {
    background: var(--success);
  }
  .status-dot.gray {
    background: var(--text-faint);
  }
  .status-dot.red {
    background: var(--danger);
  }
  .sspin {
    width: 15px;
    height: 15px;
    border: 2px solid var(--accent-soft);
    border-top-color: var(--accent);
    border-radius: 50%;
    animation: sspin 0.8s linear infinite;
    flex-shrink: 0;
  }
  @keyframes sspin {
    to { transform: rotate(360deg); }
  }
  .card h3 {
    margin-bottom: 14px;
  }
  .row {
    display: flex;
    gap: 12px;
  }
  .grow {
    flex: 1;
  }
  .code-row {
    display: flex;
    gap: 8px;
  }
  .code-row input {
    flex: 1;
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
  }
  .health {
    margin-top: 12px;
    padding: 10px 14px;
    border-radius: var(--radius);
    background: rgba(var(--danger-rgb), 0.12);
    color: var(--danger-text);
    font-size: 0.85rem;
  }
  .health.ok {
    background: rgba(var(--success-rgb), 0.1);
    color: var(--success);
  }
  .health :global(svg) {
    color: currentColor;
  }
  .section {
    margin: 22px 0 10px;
  }
  .quiet {
    color: var(--text-faint);
    font-size: 0.88rem;
  }
  .list {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .peer {
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 14px 16px;
  }
  .peer-info {
    flex: 1;
  }
  .peer-name {
    font-weight: 600;
    display: flex;
    align-items: center;
    gap: 8px;
  }
</style>
