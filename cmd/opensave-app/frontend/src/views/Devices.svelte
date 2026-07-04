<script>
  import { peers, discoveredPeers, pairingRequests, toast } from '../lib/stores.js';
  import { api } from '../lib/api.js';

  let manualIp = '';
  let manualPort = 8383;
  let busy = false;

  $: pairedList = Object.values($peers).sort((a, b) => a.name.localeCompare(b.name));
  $: pairedIds = new Set(pairedList.map((p) => p.id));
  $: lanDiscovered = $discoveredPeers.filter((d) => !d.isWan && !pairedIds.has(d.id));

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

  const pairDiscovered = (d) =>
    run(() => api.post('/api/peers/pair', { address: d.address, port: d.port }), `Pairing request sent to ${d.deviceName}`);
  const pairManual = () =>
    run(() => api.post('/api/peers/pair', { address: manualIp, port: Number(manualPort) }), 'Pairing request sent');
  const approve = (req) =>
    run(() => api.post('/api/peers/approve', { peerId: req.peerId }), `Paired with ${req.deviceName}`);
  const reject = (req) => run(() => api.post('/api/peers/reject', { peerId: req.peerId }));
  const unpair = (peer) => {
    if (!confirm(`Unpair "${peer.name}"?`)) return;
    run(() => api.del(`/api/peers/${peer.id}`), `Unpaired ${peer.name}`);
  };

  const fmtTime = (t) => (t ? new Date(t).toLocaleString() : 'never');
</script>

<div class="head">
  <h2 class="page-title">Devices</h2>
</div>

{#if $pairingRequests.length > 0}
  <div class="card requests">
    <h3>Pairing requests</h3>
    {#each $pairingRequests as req (req.peerId)}
      <div class="req-row">
        <div class="req-info">
          <strong>{req.deviceName}</strong>
          <span class="req-meta">{req.isWan ? 'via internet relay' : `${req.address}:${req.port}`}</span>
        </div>
        <button class="btn small" disabled={busy} on:click={() => reject(req)}>Ignore</button>
        <button class="btn small primary" disabled={busy} on:click={() => approve(req)}>Approve</button>
      </div>
    {/each}
  </div>
{/if}

<h3 class="section">Paired devices</h3>
{#if pairedList.length === 0}
  <div class="empty">
    <h3>No paired devices</h3>
    <p>Devices on your Wi-Fi appear below automatically. For syncing over the internet, use Internet Sync.</p>
  </div>
{:else}
  <div class="list">
    {#each pairedList as peer (peer.id)}
      <div class="card peer">
        <div class="peer-icon">{peer.deviceType === 'deck' ? '🎮' : '🖥️'}</div>
        <div class="peer-info">
          <div class="peer-name">
            {peer.name}
            <span class="badge" class:online={peer.status === 'online'} class:offline={peer.status !== 'online'}>
              {peer.status}
            </span>
          </div>
          <div class="peer-meta">
            {peer.address === 'relay' ? 'internet relay' : `${peer.address}:${peer.port}`}
            · last synced {fmtTime(peer.lastSynced)}
          </div>
        </div>
        <button class="btn small danger" disabled={busy} on:click={() => unpair(peer)}>Unpair</button>
      </div>
    {/each}
  </div>
{/if}

<h3 class="section">On your network</h3>
{#if lanDiscovered.length === 0}
  <p class="quiet">No unpaired devices found on the local network. Make sure OpenSave is running on the other device.</p>
{:else}
  <div class="list">
    {#each lanDiscovered as d (d.id)}
      <div class="card peer">
        <div class="peer-icon">{d.deviceType === 'deck' ? '🎮' : '🖥️'}</div>
        <div class="peer-info">
          <div class="peer-name">{d.deviceName}</div>
          <div class="peer-meta">{d.address}:{d.port}</div>
        </div>
        <button class="btn small primary" disabled={busy} on:click={() => pairDiscovered(d)}>Pair</button>
      </div>
    {/each}
  </div>
{/if}

<h3 class="section">Add by IP address</h3>
<div class="card manual">
  <input placeholder="192.168.1.42" bind:value={manualIp} />
  <input class="port" type="number" bind:value={manualPort} />
  <button class="btn primary" disabled={!manualIp || busy} on:click={pairManual}>Send pairing request</button>
</div>

<style>
  .head {
    margin-bottom: 20px;
  }
  .section {
    margin: 22px 0 10px;
  }
  .requests {
    border-color: rgba(138, 99, 244, 0.45);
    margin-bottom: 8px;
  }
  .requests h3 {
    margin-bottom: 10px;
  }
  .req-row {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 0;
  }
  .req-info {
    flex: 1;
    display: flex;
    flex-direction: column;
  }
  .req-meta {
    font-size: 0.78rem;
    color: var(--text-faint);
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
    min-width: 0;
  }
  .peer-name {
    font-weight: 600;
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .peer-meta {
    font-size: 0.78rem;
    color: var(--text-faint);
    margin-top: 2px;
  }
  .quiet {
    color: var(--text-faint);
    font-size: 0.88rem;
  }
  .manual {
    display: flex;
    gap: 10px;
    padding: 14px;
  }
  .manual input {
    padding: 8px 12px;
    background: var(--bg);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius);
    color: var(--text);
    outline: none;
    flex: 1;
  }
  .manual .port {
    flex: 0 0 90px;
  }
</style>
