<script>
  // Linking this game with the same game tracked under another name, and
  // stopping tracking it.
  import { games, askConfirm } from '../../lib/stores.js';
  import { api } from '../../lib/api.js';
  import { untrackGame } from '../../lib/gameactions.js';
  import { t } from '../../lib/i18n.js';
  import Link from 'lucide-svelte/icons/link';
  import LinkPicker from './LinkPicker.svelte';

  export let game;
  export let runner;
  const { busy, run } = runner;

  // Linked copies — cross-device "same game" links (the manual counterpart
  // to App-ID matching).
  let aliases = [];
  let linkTarget = '';
  $: otherGames = Object.values($games).filter((g) => g.id !== game.id);
  $: chosenName = $games[linkTarget]?.name ?? peerGames.find((g) => g.id === linkTarget)?.name ?? linkTarget;

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
      ? $t('game.manage.linkRemoteMsg', { name: remote.name, peer: remote.peerName, game: game.name })
      : $t('game.manage.linkLocalMsg', { name: other?.name ?? linkTarget, game: game.name });

    const ok = await askConfirm(message, { title: $t('game.manage.linkTitle'), confirmText: $t('game.manage.linkConfirm') });
    if (!ok) return;
    const canonicalId = game.id;
    await run($t('game.manage.linked'), () => api.post(`/api/games/${canonicalId}/link`, { alias: linkTarget }));
    linkTarget = '';
    await loadAliases();
  }

  async function unlink(aliasId) {
    await run($t('game.manage.unlinked'), () => api.del(`/api/games/${game.id}/alias/${aliasId}`));
    await loadAliases();
  }

  // Undone from the toast rather than confirmed first; see lib/undo.js.
  const untrack = () => untrackGame(game);
</script>

<div class="card">
  <h3>{$t('game.manage.linkedCopies')}</h3>
  <p class="desc">
    {$t('game.manage.linkedDesc')}
  </p>
  {#if aliases.length > 0}
    <div class="alias-list">
      {#each aliases as a}
        <div class="alias-row">
          <span class="alias-id" title={a.savePath || a.id}>
            <Link size={13} class="inline-icon" />{a.name || a.id}{a.savePath ? ` — ${a.savePath}` : ''}
          </span>
          <button class="btn small" disabled={$busy} on:click={() => unlink(a.id)}>{$t('game.manage.unlink')}</button>
        </div>
      {/each}
    </div>
  {/if}
  {#if otherGames.length > 0 || peerGames.length > 0}
    <!-- By cover, with where each copy lives underneath: same-named entries
         are normal now that one game can be tracked at several save
         locations, and a name alone could not tell them apart. -->
    <LinkPicker localGames={otherGames} {peerGames} bind:selected={linkTarget} />
    <div class="link-row">
      <button class="btn small primary" disabled={$busy || !linkTarget} on:click={linkGame}>
        <Link size={13} />{linkTarget ? $t('game.manage.linkThis', { name: chosenName }) : $t('game.manage.chooseToLink')}
      </button>
    </div>
    {#if peerGamesLoading}
      <p class="desc">{$t('game.manage.checkingPeers')}</p>
    {/if}
  {:else if peerGamesLoading}
    <p class="desc">{$t('game.manage.checkingPeers')}</p>
  {:else}
    <p class="desc">
      {$t('game.manage.nothingToLink')}
    </p>
  {/if}
</div>

<div class="card" style="margin-top: 16px;">
  <h3>{$t('game.manage.stopTracking')}</h3>
  <p class="desc">
    {$t('game.manage.stopTrackingDesc', { name: game.name })}
  </p>
  <button class="btn danger" disabled={$busy} on:click={untrack}>{$t('game.manage.stopTrackingBtn')}</button>
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

</style>
