<script>
  import { games } from '../lib/stores.js';
  import { createRunner } from '../lib/runner.js';
  import GameHeader from './game/GameHeader.svelte';
  import SnapshotsTab from './game/SnapshotsTab.svelte';
  import BranchesTab from './game/BranchesTab.svelte';
  import CloudTab from './game/CloudTab.svelte';
  import ConfigTab from './game/ConfigTab.svelte';
  import ManageTab from './game/ManageTab.svelte';

  export let params = {};

  $: game = $games[params.gameId];

  // One action at a time across the whole page — see lib/runner.js.
  const runner = createRunner();

  const tabs = [
    { id: 'snapshots', label: 'Snapshots', component: SnapshotsTab },
    { id: 'branches', label: 'Branches', component: BranchesTab },
    { id: 'cloud', label: '☁️ Cloud', component: CloudTab },
    { id: 'config', label: 'Configuration', component: ConfigTab },
    { id: 'danger', label: 'Manage', component: ManageTab }
  ];
  let tab = 'snapshots';

  // A tab is built the first time it is opened and then kept, hidden, while
  // another is shown: half-typed configuration or an open file list should
  // still be there after a look at another tab. Each tab loads what it needs
  // when it is built, so one never opened costs nothing.
  //
  // This page is reused, not rebuilt, when moving from one game to another,
  // so all of it starts over when the game changes — otherwise one game's
  // cloud list or unsaved edits would show up on the next.
  let opened = new Set([tab]);
  let shownFor = params.gameId;
  $: if (params.gameId !== shownFor) {
    shownFor = params.gameId;
    tab = 'snapshots';
    opened = new Set([tab]);
  }
  $: if (!opened.has(tab)) opened = new Set(opened).add(tab);
</script>

{#if !game}
  <div class="empty"><h3>Game not found</h3></div>
{:else}
  {#key params.gameId}
    <GameHeader {game} {runner} />

    <div class="pill-tabs tabs">
      {#each tabs as t (t.id)}
        <button class:active={tab === t.id} on:click={() => (tab = t.id)}>{t.label}</button>
      {/each}
    </div>

    {#each tabs as t (t.id)}
      {#if opened.has(t.id)}
        <div hidden={tab !== t.id}>
          <svelte:component this={t.component} {game} {runner} />
        </div>
      {/if}
    {/each}
  {/key}
{/if}

<style>
  .tabs {
    margin-bottom: 18px;
  }
</style>
