<script>
  import { fly as flyIn } from 'svelte/transition';
  import { gated } from '../lib/motion.js';
  // Instant with animations off; see lib/motion.js.
  const fly = gated(flyIn);
  import { pairingRequests, toast } from '../lib/stores.js';
  import { api } from '../lib/api.js';
  import { demandAttention } from '../lib/notify.js';
  import { t, segments } from '../lib/i18n.js';
  import Link from 'lucide-svelte/icons/link';
  import CheckDigits from './ui/CheckDigits.svelte';

  let busy = false;
  let seen = new Set(); // request ids we've already chimed for

  // React to newly-arrived requests: chime + surface the window so the
  // banner can't be missed when OpenSave is minimised or hidden to tray.
  $: onRequests($pairingRequests);
  function onRequests(list) {
    const fresh = list.filter((r) => !seen.has(r.peerId));
    if (fresh.length > 0) {
      const who = fresh[0].deviceName ?? $t('pairing.aDevice');
      demandAttention('pairing', { title: $t('pairing.wantsToPair', { device: who }), body: $t('pairing.openToAccept'), open: { view: 'devices', params: {} } });
      toast($t('pairing.wantsToPair', { device: who }), 'info');
    }
    seen = new Set(list.map((r) => r.peerId));
  }

  // The approval carries the numbers on screen, so a request replaced by a
  // newer one a moment ago is not approved unseen (409).
  const approve = (req) => act('/api/peers/approve', { peerId: req.peerId, checkCode: req.checkCode ?? '' }, $t('pairing.pairedWith', { device: req.deviceName }));
  const reject = (req) => act('/api/peers/reject', { peerId: req.peerId }, null);

  async function act(path, body, okMsg) {
    if (busy) return;
    busy = true;
    try {
      await api.post(path, body);
      if (okMsg) toast(okMsg, 'success');
    } catch (e) {
      toast(e.status === 409 ? $t('pairing.checkChanged') : e.message, 'error');
    } finally {
      busy = false;
    }
  }
  const who = (req) => req.deviceName ?? $t('pairing.aDevice');
</script>

{#if $pairingRequests.length > 0}
  <!-- In: down from the top, there from its first frame. Out: back up,
       fading as it goes. -->
  <div class="pair-wrap" in:fly={{ y: -90, duration: 320, opacity: 1 }} out:fly={{ y: -90, duration: 220 }}>
    {#each $pairingRequests as req (req.peerId)}
      <div class="pair-card" in:fly={{ y: -20, duration: 200, opacity: 1 }} out:fly={{ y: -20, duration: 180 }}>
        <div class="pair-main">
          <div class="pair-icon"><Link size={19} /></div>
          <div class="pair-body">
            <div class="pair-title">
              {#each segments($t('pairing.wantsToPair'), { device: who(req) }, ['device']) as part}{#if part.bold}<strong>{part.text}</strong>{:else}{part.text}{/if}{/each}
            </div>
            <div class="pair-sub">{req.isWan ? $t('pairing.subInternet') : $t('pairing.subFrom', { address: req.address })}</div>
          </div>
          <div class="pair-actions">
            <button class="btn small" disabled={busy} on:click={() => reject(req)}>{$t('pairing.ignore')}</button>
            <button class="btn small primary" disabled={busy} on:click={() => approve(req)}>{$t('pairing.approve')}</button>
          </div>
        </div>
        {#if req.checkCode}
          <div class="pair-check">
            <CheckDigits code={req.checkCode} size="md" />
            <p>{$t('pairing.checkMatch', { device: who(req) })}</p>
          </div>
        {:else if req.checks}
          <div class="pair-check quiet"><p>{$t('pairing.checkPending')}</p></div>
        {:else if !req.isWan}
          <div class="pair-check quiet"><p>{$t('pairing.noCheck', { device: who(req) })}</p></div>
        {/if}
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
    flex-direction: column;
    gap: 12px;
    padding: 13px 16px;
    background: var(--bg-raised);
    border: 1px solid var(--accent);
    border-radius: var(--radius-lg);
    /* Still, no pulse: the outline, the chime and the window coming to the
       front already say someone is asking. */
    box-shadow: 0 12px 40px rgba(0, 0, 0, 0.55);
  }
  .pair-main {
    display: flex;
    align-items: center;
    gap: 14px;
  }
  .pair-check {
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 10px 12px;
    border-radius: calc(12px * var(--round, 1));
    background: var(--bg);
    border: 1px solid var(--border);
  }
  .pair-check p {
    font-size: var(--text-md);
    color: var(--text-dim);
    line-height: 1.4;
  }
  .pair-check.quiet {
    padding: 8px 12px;
  }
  .pair-check.quiet p {
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
  .pair-icon {
    flex-shrink: 0;
    width: 36px;
    height: 36px;
    border-radius: calc(10px * var(--round, 1));
    display: grid;
    place-items: center;
    background: var(--accent-soft);
    color: var(--accent-text);
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
