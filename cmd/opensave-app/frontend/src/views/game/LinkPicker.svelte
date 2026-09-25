<script>
  // Choosing the game to link, by its cover: the games on this device, and
  // the ones on your other devices, as the library shows them. A name alone
  // was a long list of look-alikes — the same title tracked twice reads the
  // same until you see where each one lives.
  import Check from 'lucide-svelte/icons/check';
  import Search from 'lucide-svelte/icons/search';
  import CoverImage from '../../components/CoverImage.svelte';
  import { gameCover } from '../../lib/api.js';

  /** Games tracked here, other than this one. */
  export let localGames = [];
  /** Games on paired devices: {id, name, appId, peerName}. */
  export let peerGames = [];
  /** The chosen game's id. */
  export let selected = '';

  let query = '';
  const initials = (name = '') => name.split(/\s+/).map((w) => w[0]).join('').slice(0, 2).toUpperCase();
  const matches = (g) => !query || g.name.toLowerCase().includes(query.toLowerCase());

  // Titles that appear under the same name elsewhere first: the likeliest
  // copies of this game are at the top.
  $: local = localGames.filter(matches).sort((a, b) => a.name.localeCompare(b.name));
  $: remote = peerGames.filter(matches).sort((a, b) => a.name.localeCompare(b.name));
  $: many = localGames.length + peerGames.length > 8;
</script>

{#if many}
  <label class="search">
    <Search size={14} />
    <input type="search" placeholder="Find a game…" bind:value={query} aria-label="Find a game to link" />
  </label>
{/if}

{#each [['On this device', 'Its entry here merges into this one', local, false], ['On your other devices', 'Nothing is removed on either device', remote, true]] as [title, note, list, onPeer]}
  {#if list.length}
    <div class="group-head">
      <span class="title">{title}</span><span class="note">{note}</span>
    </div>
    <div class="grid" role="radiogroup" aria-label={title}>
      {#each list as g (g.id + (g.peerName ?? ''))}
        <button
          class="card-pick"
          role="radio"
          aria-checked={selected === g.id}
          class:on={selected === g.id}
          title={onPeer ? `${g.name} on ${g.peerName}` : `${g.name} — ${g.savePath}`}
          on:click={() => (selected = selected === g.id ? '' : g.id)}
        >
          <span class="cover">
            <span class="initials">{initials(g.name)}</span>
            <CoverImage src={gameCover(g, true)} alt="" />
            {#if selected === g.id}<span class="tick"><Check size={14} strokeWidth={3} /></span>{/if}
          </span>
          <span class="name">{g.name}</span>
          <span class="where">{onPeer ? g.peerName : g.savePath}</span>
        </button>
      {/each}
    </div>
  {/if}
{/each}
{#if many && local.length === 0 && remote.length === 0}
  <p class="none">No game matches “{query}”.</p>
{/if}

<style>
  .search {
    display: flex;
    align-items: center;
    gap: 8px;
    max-width: 280px;
    margin-top: 12px;
    padding: 6px 10px;
    border: 1px solid var(--border-strong);
    border-radius: 8px;
    background: var(--bg);
    color: var(--text-faint);
  }
  .search input {
    flex: 1;
    min-width: 0;
    border: none;
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: 0.86rem;
    outline: none;
  }
  .group-head {
    display: flex;
    align-items: baseline;
    gap: 10px;
    margin: 16px 0 8px;
  }
  .title {
    font-size: 0.8rem;
    font-weight: 600;
    color: var(--text-dim);
  }
  .note {
    font-size: 0.76rem;
    color: var(--text-faint);
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(104px, 1fr));
    gap: 12px;
  }
  .card-pick {
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding: 0;
    border: none;
    background: transparent;
    color: var(--text);
    font: inherit;
    text-align: left;
    cursor: pointer;
    min-width: 0;
  }
  .cover {
    position: relative;
    aspect-ratio: 2 / 3;
    border-radius: 8px;
    overflow: hidden;
    background: var(--bg-hover);
    display: grid;
    place-items: center;
    outline: 2px solid transparent;
    outline-offset: 2px;
    transition: outline-color 0.12s, transform 0.12s;
  }
  .card-pick:hover .cover {
    transform: translateY(-2px);
  }
  .card-pick.on .cover {
    outline-color: var(--accent);
  }
  .cover :global(img) {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .initials {
    font-size: 1.1rem;
    font-weight: 700;
    color: var(--text-faint);
  }
  .tick {
    position: absolute;
    top: 6px;
    right: 6px;
    width: 22px;
    height: 22px;
    border-radius: 50%;
    display: grid;
    place-items: center;
    background: var(--accent);
    color: #fff;
  }
  .name {
    font-size: 0.8rem;
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .where {
    font-size: 0.72rem;
    color: var(--text-faint);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .none {
    font-size: 0.84rem;
    color: var(--text-faint);
  }
</style>
