<script>
  // ── Games a peer syncs that this device has no folder for ─────────
  //
  // Only ever populated when "Ask me where to keep it" is chosen in Settings;
  // with the default an unknown game is tracked automatically and never
  // reaches this list. Shown at the top of the page on purpose: an offer
  // nobody notices is worse than a folder guessed slightly wrong, because a
  // wrong guess is at least visible and can be moved afterwards.
  import { gameList, toast, askConfirm } from '../../lib/stores.js';
  import { api, native } from '../../lib/api.js';

  let offeredGames = [];
  async function load() {
    try {
      offeredGames = (await api.get('/api/offered-games')) ?? [];
    } catch {
      offeredGames = [];
    }
  }
  // Refresh with the game list: placing or declining broadcasts a games
  // update, and so does a peer offering something new. The first run of this
  // statement is the initial load.
  $: if ($gameList) load();

  async function place(offer) {
    const dir = await native.selectDirectory(`Folder for "${offer.name}" on this device`);
    if (!dir) return;
    try {
      await api.post(`/api/offered-games/${offer.gameId}/place`, { path: dir });
      toast(`${offer.name} is now tracked here`, 'success');
      await load();
    } catch (e) {
      toast(e.message, 'error');
    }
  }

  async function decline(offer) {
    if (
      !(await askConfirm(
        `Stop being asked about "${offer.name}"?

It will not sync to this device. You can still track it yourself later, which undoes this.`,
        { title: 'Decline this game?', confirmText: 'Decline' }
      ))
    )
      return;
    try {
      await api.post(`/api/offered-games/${offer.gameId}/decline`);
      await load();
    } catch (e) {
      toast(e.message, 'error');
    }
  }
</script>

{#if offeredGames.length > 0}
  <div class="card offers">
    <h3>Waiting for a folder</h3>
    <p class="intro">
      {offeredGames.length === 1 ? 'Another device syncs this game' : 'Other devices sync these games'},
      but this one doesn't know where to keep
      {offeredGames.length === 1 ? 'it' : 'them'} yet. Nothing syncs until you choose.
    </p>
    {#each offeredGames as offer (offer.gameId + offer.peerId)}
      <div class="row">
        <div class="info">
          <strong>{offer.name}</strong>
          <span class="hint">
            Kept at <code>{offer.peerPath}</code> on the other device.
          </span>
        </div>
        <div class="actions">
          <button class="btn primary" on:click={() => place(offer)}>Choose folder…</button>
          <button class="btn" on:click={() => decline(offer)}>Decline</button>
        </div>
      </div>
    {/each}
  </div>
{/if}

<style>
  /* Given the accent border because being noticed IS the feature: an offer
     nobody sees leaves a save silently not syncing, which is worse than a
     folder guessed wrong. */
  .offers {
    border-left: 3px solid var(--accent);
  }
  .intro {
    color: var(--text-dim);
    font-size: 0.88rem;
    margin: 0 0 12px;
  }
  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 10px 0;
    border-top: 1px solid var(--border);
  }
  .info {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }
  .info code {
    word-break: break-all;
  }
  .actions {
    display: flex;
    gap: 8px;
    flex-shrink: 0;
  }
</style>
