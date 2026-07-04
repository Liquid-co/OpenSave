<script>
  import { conflicts, games, toast } from '../lib/stores.js';
  import { api } from '../lib/api.js';

  let busy = false;

  $: entries = Object.entries($conflicts);
  $: current = entries[0]; // one at a time
  $: gameName = current ? ($games[current[0]]?.name ?? current[0]) : '';

  async function resolve(resolution) {
    if (!current || busy) return;
    busy = true;
    const [gameId, conflict] = current;
    try {
      const res = await api.post(`/api/games/${gameId}/resolve-conflict`, {
        peerId: conflict.peer.ID ?? conflict.peer.id,
        resolution
      });
      toast(
        resolution === 'merge-branch'
          ? `Both versions kept — remote saved to branch "${res.branchName}"`
          : `Conflict resolved (${resolution})`,
        'success'
      );
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      busy = false;
    }
  }

  const fmtTime = (t) => (t ? new Date(t).toLocaleString() : '—');
</script>

{#if current}
  <div class="overlay">
    <div class="modal card">
      <h3>Save conflict — {gameName}</h3>
      <p class="desc">
        Both this device and <strong>{current[1].peer.Name ?? current[1].peer.name}</strong> changed this save
        since the last sync. Which version do you want to keep?
      </p>

      <div class="versions">
        <div class="version">
          <div class="v-title">This device</div>
          <div class="v-meta">{current[1].localSnap.comment}</div>
          <div class="v-time">{fmtTime(current[1].localSnap.timestamp)}</div>
        </div>
        <div class="version">
          <div class="v-title">{current[1].peer.Name ?? current[1].peer.name}</div>
          <div class="v-meta">{current[1].remoteSnap.comment}</div>
          <div class="v-time">{fmtTime(current[1].remoteSnap.timestamp)}</div>
        </div>
      </div>

      <div class="actions">
        <button class="btn" disabled={busy} on:click={() => resolve('keep-local')}>Keep mine</button>
        <button class="btn" disabled={busy} on:click={() => resolve('keep-remote')}>Keep theirs</button>
        <button class="btn primary" disabled={busy} on:click={() => resolve('merge-branch')}>
          Keep both (new branch)
        </button>
      </div>
      <p class="hint-line">
        “Keep both” saves the other device's version on a separate branch — nothing is lost, and you can
        switch between them any time.
      </p>
    </div>
  </div>
{/if}

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.6);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 90;
  }
  .modal {
    width: 520px;
    max-width: calc(100vw - 48px);
  }
  h3 {
    margin-bottom: 8px;
  }
  .desc {
    color: var(--text-dim);
    font-size: 0.9rem;
    margin-bottom: 16px;
  }
  .versions {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 10px;
    margin-bottom: 18px;
  }
  .version {
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 12px;
  }
  .v-title {
    font-weight: 600;
    margin-bottom: 4px;
  }
  .v-meta {
    font-size: 0.8rem;
    color: var(--text-dim);
    margin-bottom: 4px;
  }
  .v-time {
    font-size: 0.75rem;
    color: var(--text-faint);
  }
  .actions {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
  }
  .hint-line {
    margin-top: 12px;
    font-size: 0.78rem;
    color: var(--text-faint);
  }
</style>
