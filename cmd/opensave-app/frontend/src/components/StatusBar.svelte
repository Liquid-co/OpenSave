<script>
  import { wsConnected, syncActivity, wanRoom, peers } from '../lib/stores.js';

  $: running = Object.entries($syncActivity).filter(([, s]) => s.state === 'running');
  $: onlinePeers = Object.values($peers).filter((p) => p.status === 'online').length;
  $: statusText = running.length
    ? `Syncing ${running.length} game${running.length > 1 ? 's' : ''}…`
    : 'No syncs in progress';
</script>

<footer>
  <div class="left">
    <span class="dot" class:green={$wsConnected} class:gray={!$wsConnected}></span>
    <span>{statusText}</span>
    {#if running.length && running[0][1].percentage != null}
      <span class="pct">{running[0][1].percentage}%</span>
    {/if}
  </div>
  <div class="right">
    {#if $wanRoom?.connected}
      <span class="wan">relay: {$wanRoom.roomCode}</span>
    {/if}
    <span>{onlinePeers} peer{onlinePeers === 1 ? '' : 's'} online</span>
    <span class="ver">OpenSave v2.0.0</span>
  </div>
</footer>

<style>
  footer {
    height: var(--statusbar-h);
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0 14px;
    background: var(--bg-sidebar);
    border-top: 1px solid var(--border);
    font-size: 0.76rem;
    color: var(--text-faint);
    flex-shrink: 0;
  }
  .left,
  .right {
    display: flex;
    align-items: center;
    gap: 12px;
  }
  .pct {
    color: var(--accent);
    font-weight: 600;
  }
  .wan {
    color: var(--success);
  }
  .ver {
    opacity: 0.7;
  }
</style>
