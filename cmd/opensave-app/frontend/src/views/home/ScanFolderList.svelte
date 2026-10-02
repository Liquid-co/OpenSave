<script>
  // Every folder one game was found in, opened under its tile in the scan.
  // Ticking one fires `toggle` with its id.
  import { createEventDispatcher } from 'svelte';
  import { contentsLabel, normPath } from '../../lib/scan.js';
  import { t, segments } from '../../lib/i18n.js';

  export let group;
  export let selected = new Set();
  /** Normalised save paths already tracked. */
  export let trackedPaths = new Set();

  const dispatch = createEventDispatcher();
</script>

<div class="panel">
  <div class="head">
    {#each segments($t('home.scan.folderList.headFound'), { name: group.primary.name, n: group.members.length }, ['name']) as part}{#if part.bold}<strong>{part.text}</strong>{:else}{part.text}{/if}{/each}
    {$t('home.scan.folderList.headTick')}
  </div>
  {#each group.members as m (m.id)}
    {@const tracked = trackedPaths.has(normPath(m.savePath))}
    <label class="row" class:covered={m.role === 'inside'}>
      <input
        type="checkbox"
        checked={selected.has(m.id)}
        disabled={m.role === 'inside' || tracked}
        on:change={() => dispatch('toggle', m.id)}
      />
      <span class="main">
        <span class="path">{m.savePath}</span>
        <span class="meta">
          {contentsLabel(m)}
          {#if m.role === 'primary'}<span class="tag tag-primary">{$t('home.scan.folderList.tagSave')}</span>
          {:else if m.role === 'location'}<span class="tag tag-loc">{$t('home.scan.folderList.tagLocation')}</span>
          {:else if m.role === 'inside'}<span class="tag">{$t('home.scan.folderList.tagInside')}</span>
          {:else if m.role === 'alternative'}<span class="tag tag-alt">{$t('home.scan.folderList.tagAlt')}</span>{/if}
          {#if tracked}<span class="tag tag-primary">{$t('home.scan.folderList.tagTracked')}</span>{/if}
        </span>
      </span>
    </label>
  {/each}
</div>

<style>
  /* Spans the grid so the folder list reads as a list, not as a tile. */
  .panel {
    grid-column: 1 / -1;
    background: var(--bg-elev, rgba(127, 127, 127, 0.08));
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 12px 14px;
    margin: 2px 0 10px;
  }
  .head {
    font-size: 0.8rem;
    color: var(--text-dim);
    margin-bottom: 10px;
  }
  .row {
    display: flex;
    align-items: flex-start;
    gap: 9px;
    padding: 6px 4px;
    border-top: 1px solid var(--border);
    cursor: pointer;
  }
  .row:first-of-type {
    border-top: 0;
  }
  .row.covered {
    opacity: 0.55;
    cursor: default;
  }
  .main {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }
  .path {
    font-size: 0.78rem;
    color: var(--text);
    word-break: break-all;
  }
  .meta {
    font-size: 0.72rem;
    color: var(--text-faint);
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
  }
  .tag {
    border: 1px solid var(--border);
    border-radius: 999px;
    padding: 0 7px;
    font-size: 0.68rem;
    line-height: 1.5;
    color: var(--text-dim);
  }
  .tag-primary {
    border-color: var(--accent);
    color: var(--accent-text);
  }
  .tag-loc {
    border-color: var(--ok, var(--accent));
    color: var(--ok, var(--accent));
  }
  .tag-alt {
    border-color: var(--warn, var(--border));
    color: var(--warn, var(--text-dim));
  }
</style>
