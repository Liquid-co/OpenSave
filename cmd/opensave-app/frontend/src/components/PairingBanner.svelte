<script>
  import { fly } from 'svelte/transition';
  import { pairingRequests, toast } from '../lib/stores.js';
  import { api, native } from '../lib/api.js';

  let busy = false;
  let seen = new Set(); // request ids we've already chimed for

  // React to newly-arrived requests: chime + surface the window so the
  // banner can't be missed when OpenSave is minimised or hidden to tray.
  $: onRequests($pairingRequests);
  function onRequests(list) {
    const fresh = list.filter((r) => !seen.has(r.peerId));
    if (fresh.length > 0) {
      playChime();
      native.showWindow();
      const who = fresh[0].deviceName ?? 'A device';
      toast(`${who} wants to pair`, 'info');
    }
    seen = new Set(list.map((r) => r.peerId));
  }

  const approve = (req) => act('/api/peers/approve', req, `Paired with ${req.deviceName}`);
  const reject = (req) => act('/api/peers/reject', req, null);

  async function act(path, req, okMsg) {
    if (busy) return;
    busy = true;
    try {
      await api.post(path, { peerId: req.peerId });
      if (okMsg) toast(okMsg, 'success');
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      busy = false;
    }
  }

  // A short, pleasant two-note chime synthesised on the fly (no asset).
  function playChime() {
    try {
      const Ctx = window.AudioContext || window.webkitAudioContext;
      if (!Ctx) return;
      const ctx = new Ctx();
      if (ctx.state === 'suspended') ctx.resume();
      const now = ctx.currentTime;
      [660, 990].forEach((freq, i) => {
        const osc = ctx.createOscillator();
        const gain = ctx.createGain();
        osc.type = 'sine';
        osc.frequency.value = freq;
        osc.connect(gain);
        gain.connect(ctx.destination);
        const t = now + i * 0.13;
        gain.gain.setValueAtTime(0, t);
        gain.gain.linearRampToValueAtTime(0.18, t + 0.02);
        gain.gain.exponentialRampToValueAtTime(0.0001, t + 0.35);
        osc.start(t);
        osc.stop(t + 0.36);
      });
      setTimeout(() => ctx.close(), 900);
    } catch {}
  }

  const source = (req) => (req.isWan ? '🌐 over the internet' : `🖧 ${req.address}`);
</script>

{#if $pairingRequests.length > 0}
  <div class="pair-wrap" transition:fly={{ y: -90, duration: 320 }}>
    {#each $pairingRequests as req (req.peerId)}
      <div class="pair-card" transition:fly={{ y: -20, duration: 200 }}>
        <div class="pair-icon">🔗</div>
        <div class="pair-body">
          <div class="pair-title"><strong>{req.deviceName ?? 'A device'}</strong> wants to pair</div>
          <div class="pair-sub">{source(req)} · approve to start syncing your saves</div>
        </div>
        <div class="pair-actions">
          <button class="btn small" disabled={busy} on:click={() => reject(req)}>Ignore</button>
          <button class="btn small primary" disabled={busy} on:click={() => approve(req)}>Approve</button>
        </div>
      </div>
    {/each}
  </div>
{/if}

<style>
  .pair-wrap {
    position: fixed;
    top: calc(var(--titlebar-h) + 10px);
    left: 50%;
    transform: translateX(-50%);
    z-index: 120;
    display: flex;
    flex-direction: column;
    gap: 8px;
    width: min(520px, calc(100vw - 40px));
  }
  .pair-card {
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 13px 16px;
    background: var(--bg-raised);
    border: 1px solid var(--accent);
    border-radius: var(--radius-lg);
    box-shadow: 0 12px 40px rgba(0, 0, 0, 0.55), 0 0 0 1px var(--accent-soft);
    animation: pair-glow 2s ease-in-out infinite;
  }
  @keyframes pair-glow {
    0%, 100% { box-shadow: 0 12px 40px rgba(0, 0, 0, 0.55), 0 0 0 1px var(--accent-soft); }
    50% { box-shadow: 0 12px 44px rgba(0, 0, 0, 0.6), 0 0 0 3px var(--accent-soft); }
  }
  .pair-icon {
    font-size: 1.5rem;
    flex-shrink: 0;
  }
  .pair-body {
    flex: 1;
    min-width: 0;
  }
  .pair-title {
    font-size: 0.95rem;
  }
  .pair-sub {
    font-size: 0.78rem;
    color: var(--text-faint);
    margin-top: 2px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .pair-actions {
    display: flex;
    gap: 8px;
    flex-shrink: 0;
  }
</style>
