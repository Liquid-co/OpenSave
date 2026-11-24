<script>
  import { t as i18n } from '../lib/i18n.js';
  import { slidingIndicator } from '../lib/motion.js';
  import { games } from '../lib/stores.js';
  import { createRunner } from '../lib/runner.js';
  import GameHeader from './game/GameHeader.svelte';
  import SnapshotsTab from './game/SnapshotsTab.svelte';
  import BranchesTab from './game/BranchesTab.svelte';
  import CloudTab from './game/CloudTab.svelte';
  import ConfigTab from './game/ConfigTab.svelte';
  import ManageTab from './game/ManageTab.svelte';
  import History from 'lucide-svelte/icons/history';
  import GitBranch from 'lucide-svelte/icons/git-branch';
  import Cloud from 'lucide-svelte/icons/cloud';
  import SlidersHorizontal from 'lucide-svelte/icons/sliders-horizontal';
  import Wrench from 'lucide-svelte/icons/wrench';

  export let params = {};

  $: game = $games[params.gameId];

  // One action at a time across the whole page — see lib/runner.js.
  const runner = createRunner();

  const tabs = [
    { id: 'snapshots', label: 'game.tab.snapshots', icon: History, component: SnapshotsTab },
    { id: 'branches', label: 'game.tab.branches', icon: GitBranch, component: BranchesTab },
    { id: 'cloud', label: 'game.tab.cloud', icon: Cloud, component: CloudTab },
    { id: 'config', label: 'game.tab.config', icon: SlidersHorizontal, component: ConfigTab },
    { id: 'danger', label: 'game.tab.manage', icon: Wrench, component: ManageTab }
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
  // Opened on a snapshot (from Ctrl+K): the list it is in.
  $: if (params.snapshot) tab = 'snapshots';

  let shownFor = params.gameId;
  $: if (params.gameId !== shownFor) {
    shownFor = params.gameId;
    tab = 'snapshots';
    opened = new Set([tab]);
  }
  $: if (!opened.has(tab)) opened = new Set(opened).add(tab);
</script>

{#if !game}
  <div class="empty"><h3>{$i18n('game.notFound')}</h3></div>
{:else}
  {#key params.gameId}
    <GameHeader {game} {runner} renameRequest={params.rename} />

    <div class="pill-tabs tabs" use:slidingIndicator={{ inset: 10 }}>
      {#each tabs as t (t.id)}
        <button class:active={tab === t.id} on:click={() => (tab = t.id)}><svelte:component this={t.icon} size={15} />{$i18n(t.label)}</button>
      {/each}
    </div>

    {#each tabs as t (t.id)}
      {#if opened.has(t.id)}
        <div hidden={tab !== t.id}>
          <svelte:component this={t.component} {game} {runner} {...t.id === 'snapshots' ? { focus: params.snapshot } : {}} />
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
