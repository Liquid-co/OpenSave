<script>
  import { settings, wanRoom, peers, toast } from '../lib/stores.js';
  import { api } from '../lib/api.js';

  let codeDraft = '';
  let relayDraft = '';
  let busy = false;
  let health = null;

  $: if ($settings && codeDraft === '' && relayDraft === '') {
    codeDraft = $settings.syncCode ?? '';
    relayDraft = $settings.relayUrl ?? '';
  }
  $: pairedIds = new Set(Object.keys($peers));
  $: roomPeers = $wanRoom?.peers ?? [];

  function randomCode() {
    const words = ['swift', 'cozy', 'pixel', 'nova', 'ember', 'lunar', 'frost', 'zen'];
    const w = () => words[Math.floor(Math.random() * words.length)];
    codeDraft = `${w()}-${w()}-${Math.floor(1000 + Math.random() * 9000)}`;
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

  const joinRoom = () =>
    run(() => api.post('/api/settings', { syncCode: codeDraft.trim(), relayUrl: relayDraft.trim() }),
      codeDraft.trim() ? 'Joining relay room…' : 'Left the relay room');
  const leaveRoom = () => {
    codeDraft = '';
    return run(() => api.post('/api/settings', { syncCode: '' }), 'Left the relay room');
  };
  const pairWan = (p) =>
    run(() => api.post('/api/peers/pair', { peerId: p.id, address: 'relay' }), `Pairing request sent to ${p.deviceName}`);

  async function checkHealth() {
    health = null;
    try {
      health = await api.get('/api/relay/health');
    } catch (e) {
      toast(e.message, 'error');
    }
  }

  function copyCode() {
    navigator.clipboard?.writeText($settings.syncCode);
    toast('Room code copied');
  }
</script>

<div class="head">
  <h2 class="page-title">Internet Sync</h2>
  {#if $wanRoom}
    <span class="badge" class:online={$wanRoom.connected} class:offline={!$wanRoom.connected}>
      {$wanRoom.connected ? 'connected' : $wanRoom.state}
    </span>
  {/if}
</div>

<p class="lead">
  Sync across the internet with no port forwarding: both devices join the same room code on a relay, and
  saves travel through an encrypted tunnel. The relay never stores your files.
</p>

<div class="card">
  <h3>Relay room</h3>
  <div class="row">
    <div class="field grow">
      <label for="room-code">Room code — share this with your other device</label>
      <div class="code-row">
        <input id="room-code" placeholder="e.g. cozy-nova-4821" bind:value={codeDraft} />
        <button class="btn" on:click={randomCode}>🎲</button>
        {#if $settings?.syncCode}
          <button class="btn" on:click={copyCode}>Copy</button>
        {/if}
      </div>
    </div>
  </div>
  <div class="row">
    <div class="field grow">
      <label for="relay-url">Relay server (self-hostable)</label>
      <input id="relay-url" bind:value={relayDraft} />
      <span class="hint">Run your own with the opensave-relay binary and point this at it.</span>
    </div>
  </div>
  <div class="actions">
    <button class="btn" on:click={checkHealth}>Test relay</button>
    {#if $settings?.syncCode}
      <button class="btn danger" disabled={busy} on:click={leaveRoom}>Leave room</button>
    {/if}
    <button class="btn primary" disabled={busy} on:click={joinRoom}>
      {$settings?.syncCode ? 'Update' : 'Join room'}
    </button>
  </div>
  {#if health}
    <div class="health" class:ok={health.reachable}>
      {#if health.reachable}
        ✓ Relay reachable — {health.health?.clients ?? 0} client(s) in {health.health?.rooms ?? 0} room(s)
      {:else}
        ✕ Relay unreachable: {health.error}
      {/if}
    </div>
  {/if}
  {#if $wanRoom?.error}
    <div class="health">✕ {$wanRoom.error}</div>
  {/if}
</div>

{#if $settings?.syncCode}
  <h3 class="section">In this room</h3>
  {#if roomPeers.length === 0}
    <p class="quiet">
      No other devices in the room yet. Enter the same code on your other device and it will appear here.
    </p>
  {:else}
    <div class="list">
      {#each roomPeers as p (p.id)}
        <div class="card peer">
          <div class="peer-icon">{p.deviceType === 'deck' ? '🎮' : '🖥️'}</div>
          <div class="peer-info">
            <div class="peer-name">
              {p.deviceName}
              <span class="badge" class:online={p.online} class:offline={!p.online}>{p.online ? 'online' : 'away'}</span>
            </div>
          </div>
          {#if p.paired || pairedIds.has(p.id)}
            <span class="badge online">paired</span>
          {:else}
            <button class="btn small primary" disabled={busy} on:click={() => pairWan(p)}>Pair</button>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
{/if}

<style>
  .head {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 12px;
  }
  .lead {
    color: var(--text-dim);
    font-size: 0.9rem;
    max-width: 640px;
    margin-bottom: 20px;
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
    background: rgba(217, 87, 87, 0.12);
    color: #f1a3a3;
    font-size: 0.85rem;
  }
  .health.ok {
    background: rgba(74, 222, 128, 0.1);
    color: var(--success);
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
  .peer-icon {
    font-size: 1.3rem;
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
