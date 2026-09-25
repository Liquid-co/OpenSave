<script>
  // Linking this game with the same game tracked under another name, and
  // stopping tracking it.
  import { games, askConfirm } from '../../lib/stores.js';
  import { api } from '../../lib/api.js';
  import { untrackGame } from '../../lib/gameactions.js';
  import Link from 'lucide-svelte/icons/link';

  export let game;
  export let runner;
  const { busy, run } = runner;

  // Linked copies — cross-device "same game" links (the manual counterpart
  // to App-ID matching).
  let aliases = [];
  let linkTarget = '';
  $: otherGames = Object.values($games).filter((g) => g.id !== game.id);

  async function loadAliases() {
    try {
      aliases = await api.get(`/api/games/${game.id}/aliases`);
    } catch {
      aliases = [];
    }
  }
  loadAliases();

  // Games tracked on paired devices. Without these the picker can only offer
  // entries from this machine, which merges local duplicates but can never
  // connect a title tracked under one name here and another name there —
  // the case App-ID matching can't cover, because a save sitting in
  // AppData\<company>\<game> carries no App ID anywhere in its path.
  let peerGames = [];
  let peerGamesLoading = false;

  async function loadPeerGames() {
    peerGamesLoading = true;
    peerGames = [];
    try {
      const peers = await api.get('/api/peers');
      // Ask each device separately and keep whatever answers: one being
      // offline must not cost you the ability to link against the others.
      const results = await Promise.allSettled(
        (peers ?? []).map(async (p) => ({
          peer: p,
          games: await api.get(`/api/peers/${p.id}/games`)
        }))
      );
      const local = new Set(Object.keys($games));
      peerGames = results
        .filter((r) => r.status === 'fulfilled')
        .flatMap((r) =>
          (r.value.games ?? [])
            // An id we already track is the same entry, not a link target.
            .filter((g) => !local.has(g.id))
            .map((g) => ({ ...g, peerName: r.value.peer.name }))
        );
    } catch {
      peerGames = [];
    } finally {
      peerGamesLoading = false;
    }
  }
  loadPeerGames();

  async function linkGame() {
    if (!linkTarget) return;
    const other = $games[linkTarget];
    const remote = peerGames.find((g) => g.id === linkTarget);

    // Two different operations behind one button, and the difference matters
    // to the user: merging a local entry removes it from this library, while
    // linking another device's entry changes nothing here at all. Saying
    // "removed from your library" for the second would be a lie about a
    // destructive step that isn't happening.
    const message = remote
      ? `Link "${remote.name}" on ${remote.peerName} to "${game.name}"? The two will be treated as the same game when these devices sync. Nothing on either device is removed.`
      : `Link "${other?.name ?? linkTarget}" into "${game.name}"? They'll be treated as the same game when syncing across devices. "${other?.name ?? linkTarget}" leaves your library here — its save files stay where they are, and its snapshots move to "${game.name}", on a branch of their own.`;

    const ok = await askConfirm(message, { title: 'Link games?', confirmText: 'Link' });
    if (!ok) return;
    const canonicalId = game.id;
    await run('Games linked', () => api.post(`/api/games/${canonicalId}/link`, { alias: linkTarget }));
    linkTarget = '';
    await loadAliases();
  }

  async function unlink(aliasId) {
    await run('Link removed', () => api.del(`/api/games/${game.id}/alias/${aliasId}`));
    await loadAliases();
  }

  // Undone from the toast rather than confirmed first; see lib/undo.js.
  const untrack = () => untrackGame(game);
</script>

<div class="card">
  <h3>Linked copies</h3>
  <p class="desc">
    If this game is tracked under a different name or drive on another PC (e.g. a Steam copy vs. a
    portable copy), link the copies so their saves sync across devices. Linking merges another tracked
    game here into this one: its save files stay where they are, and its snapshots move here, onto a
    branch named after it.
  </p>
  {#if aliases.length > 0}
    <div class="alias-list">
      {#each aliases as a}
        <div class="alias-row">
          <span class="alias-id" title={a.savePath || a.id}>
            <Link size={13} class="inline-icon" />{a.name || a.id}{a.savePath ? ` — ${a.savePath}` : ''}
          </span>
          <button class="btn small" disabled={$busy} on:click={() => unlink(a.id)}>Unlink</button>
        </div>
      {/each}
    </div>
  {/if}
  {#if otherGames.length > 0 || peerGames.length > 0}
    <div class="link-row">
      <select bind:value={linkTarget}>
        <option value="">Choose a game to link…</option>
        {#if otherGames.length > 0}
          <optgroup label="On this device (merges the entry)">
            {#each otherGames as g}
              <!-- Same-named entries are normal now that one game can be
                   tracked at several save locations, so show the path too —
                   otherwise duplicates are indistinguishable in this list. -->
              <option value={g.id}>{g.name} — {g.savePath}</option>
            {/each}
          </optgroup>
        {/if}
        {#if peerGames.length > 0}
          <optgroup label="On a paired device (nothing is removed)">
            {#each peerGames as g}
              <option value={g.id}>{g.name} — {g.peerName}</option>
            {/each}
          </optgroup>
        {/if}
      </select>
      <button class="btn small primary" disabled={$busy || !linkTarget} on:click={linkGame}>Link</button>
    </div>
    {#if peerGamesLoading}
      <p class="desc">Checking paired devices…</p>
    {/if}
  {:else if peerGamesLoading}
    <p class="desc">Checking paired devices…</p>
  {:else}
    <p class="desc">
      Nothing to link to. Track the other copy on this device, or pair the device that
      has it and make sure it's online — its games appear here once it answers.
    </p>
  {/if}
</div>

<div class="card" style="margin-top: 16px;">
  <h3>Stop tracking</h3>
  <p class="desc">
    Removes "{game.name}" from OpenSave. Your save files and existing snapshot archives on disk are
    kept.
  </p>
  <button class="btn danger" disabled={$busy} on:click={untrack}>Stop tracking this game</button>
</div>

<style>
  .desc {
    color: var(--text-dim);
    font-size: 0.88rem;
    margin: 8px 0 14px;
  }
  .alias-list {
    display: flex;
    flex-direction: column;
    gap: 6px;
    margin-bottom: 12px;
  }
  .alias-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    padding: 7px 10px;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }
  .alias-id {
    font-family: monospace;
    font-size: 0.82rem;
    color: var(--text-dim);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .link-row {
    display: flex;
    gap: 10px;
    align-items: center;
    margin-top: 14px;
  }
  .link-row select {
    flex: 1;
    min-width: 0;
    padding: 8px 10px;
    background-color: var(--bg);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius);
    color: var(--text);
  }
</style>
