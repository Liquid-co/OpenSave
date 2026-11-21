<script>
  // Devices: where this game's save lives. This PC with its save folder; each
  // paired device, when it last agreed with this one about this save and
  // which branch it was on then; the copies in the cloud; and linked copies
  // (the same game tracked under another name).
  //
  // A paired device's branch is the one it answered the last sync with, and
  // is said to be no more than that.
  import { onMount } from 'svelte';
  import { peers, settings, toast, askConfirm, navigate } from '../../lib/stores.js';
  import { api, native } from '../../lib/api.js';
  import { restoreKey } from '../../lib/toasts.js';
  import { isHandheld } from '../../lib/devices.js';
  import { providerConfigured } from '../../lib/cloudproviders.js';
  import { gameState, newestCopy, copyTime } from '../../lib/backupsview.js';
  import { timeAgoParts } from '../../lib/timeago.js';
  import { t } from '../../lib/i18n.js';
  import CopiesDialog from '../backups/CopiesDialog.svelte';
  import LinkedCopies from './LinkedCopies.svelte';
  import Gamepad2 from 'lucide-svelte/icons/gamepad-2';
  import Monitor from 'lucide-svelte/icons/monitor';
  import Cloud from 'lucide-svelte/icons/cloud';
  import FolderOpen from 'lucide-svelte/icons/folder-open';
  import ArrowUp from 'lucide-svelte/icons/arrow-up';
  import GitBranch from 'lucide-svelte/icons/git-branch';

  export let game;
  export let runner;
  const { busy, run } = runner;

  const ago = (at) => {
    const p = timeAgoParts(typeof at === 'number' ? new Date(at).toISOString() : at);
    if (p.kind === 'invalid') return p.text;
    const { kind, ...params } = p;
    return $t('timeAgo.' + kind, params);
  };

  $: paired = Object.values($peers).sort((a, b) => Number(b.status === 'online') - Number(a.status === 'online') || a.name.localeCompare(b.name));
  async function openSaveFolder() {
    const problem = await native.openFolder(game.savePath);
    if (problem) toast(problem, 'error');
  }

  // ── The cloud ──
  let cloud = null; // this game's entry in the overview, or null
  let cloudState = 'loading'; // 'loading' | 'ready' | 'off' | 'error'
  async function readCloud() {
    try {
      const cfg = (await api.get('/api/settings')).cloudSync;
      if (!cfg || !providerConfigured(cfg) || !cfg.enabled || cfg.provider === 'webhook') {
        cloudState = 'off';
        return;
      }
      const all = await api.get('/api/cloud/overview');
      cloud = all.find((g) => g.gameId === game.id) ?? null;
      cloudState = 'ready';
    } catch {
      cloudState = 'error';
    }
  }
  onMount(readCloud);
  $: st = cloud ? gameState(cloud) : 'empty';
  let copiesOpen = false;
  let sending = false;
  async function upload() {
    sending = true;
    try {
      const res = await api.post(`/api/cloud/sync-local/${game.id}`);
      toast(res.uploaded ? $t('backups.uploaded', { n: res.uploaded }) : $t('backups.upToDate'), 'success', { game: game.id, detail: game.name });
      await readCloud();
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      sending = false;
    }
  }
  async function putBack(c) {
    const when = new Date(copyTime(c)).toLocaleString(undefined, { day: 'numeric', month: 'short', hour: 'numeric', minute: '2-digit' });
    if (!(await askConfirm($t('backups.putBackBody', { game: game.name, when }), { title: $t('backups.putBackTitle', { game: game.name }), confirmText: $t('backups.putBack') }))) return;
    run($t('backups.putBackDone', { game: game.name }), () => api.post(`/api/cloud/restore/${game.id}`, { fileName: c.name }), { key: restoreKey(game.id) });
  }
</script>

<div class="cols">
  <section class="card devices" aria-labelledby="dev-title">
    <h3 id="dev-title">{$t('game.devices.title')}</h3>
    <div class="rows">
      <div class="dev here" style="--i: 0">
        <span class="tile"><svelte:component this={isHandheld($settings?.deviceType) ? Gamepad2 : Monitor} size={18} /></span>
        <div class="text">
          <div class="name">{$settings?.deviceName ?? ''}<span class="this">{$t('game.devices.thisDevice')}</span></div>
          <div class="line"><GitBranch size={13} />{$t('game.devices.plays', { branch: game.activeBranch })}</div>
          <div class="path" title={game.savePath}><bdi>{game.savePath}</bdi></div>
        </div>
        <button class="btn small" on:click={openSaveFolder}><FolderOpen size={14} />{$t('game.header.openFolder')}</button>
      </div>
      {#each paired as p, i (p.id)}
        {@const synced = game.lastSyncedWith?.[p.id]}
        {@const pb = game.peerBranches?.[p.id]}
        <div class="dev" class:offline={p.status !== 'online'} style="--i: {i + 1}">
          <span class="tile"><svelte:component this={isHandheld(p.deviceType) ? Gamepad2 : Monitor} size={18} /></span>
          <div class="text">
            <div class="name">{p.name}<span class="state">{p.status === 'online' ? $t('game.devices.online') : $t('game.devices.offline')}</span></div>
            <div class="line" class:never={!synced}>{synced ? $t('game.devices.synced', { when: ago(synced) }) : $t('game.devices.neverSynced')}</div>
            {#if pb}
              <div class="line" class:apart={pb.branch !== game.activeBranch} title={$t('game.branches.asOfSync')}>
                <GitBranch size={13} />{pb.branch === game.activeBranch ? $t('game.devices.sameBranch', { branch: pb.branch }) : $t('game.devices.otherBranch', { branch: pb.branch })}
              </div>
            {/if}
          </div>
        </div>
      {:else}
        <div class="none">
          <p>{$t('game.devices.noPeers')}</p>
          <button class="btn small" on:click={() => navigate('devices')}>{$t('devices.add.title')}</button>
        </div>
      {/each}
    </div>
  </section>

  <section class="card cloud" aria-labelledby="cloud-title">
    <h3 id="cloud-title"><Cloud size={16} />{$t('game.devices.cloudTitle')}</h3>
    {#if cloudState === 'loading'}
      <div class="skeleton" style="height: 40px"></div>
    {:else if cloudState === 'off'}
      <p class="desc">{$t('game.devices.cloudOff')}</p>
      <button class="btn small" on:click={() => navigate('cloud')}>{$t('game.devices.cloudSetUp')}</button>
    {:else if cloudState === 'error'}
      <p class="desc">{$t('backups.readFailed')}</p>
      <button class="btn small" on:click={readCloud}>{$t('backups.hero.retry')}</button>
    {:else}
      <div class="cloud-facts">
        <div><span class="cap">{$t('game.devices.copies')}</span><b>{cloud?.copies.length ?? 0}</b></div>
        <div><span class="cap">{$t('game.devices.newest')}</span><b>{cloud && newestCopy(cloud) ? ago(copyTime(newestCopy(cloud))) : $t('game.hero.notYet')}</b></div>
      </div>
      <p class="cloud-state s-{st}">
        {st === 'safe' ? $t('game.devices.cloudSafe') : st === 'behind' ? $t('backups.state.behind') : st === 'waiting' ? $t('backups.state.waiting') : $t('backups.noCopies')}
      </p>
      <div class="cloud-acts">
        <button class="btn small" disabled={!cloud?.copies.length} on:click={() => (copiesOpen = true)}>{$t('game.devices.seeCopies')}</button>
        <button class="btn small" class:sending disabled={sending || $busy} on:click={upload}><span class="arrow"><ArrowUp size={14} /></span>{sending ? $t('backups.sending') : $t('backups.uploadMissing')}</button>
      </div>
    {/if}
  </section>
</div>

<div class="linked-wrap">
  <LinkedCopies {game} {runner} />
</div>

{#if copiesOpen && cloud}
  <CopiesDialog game={cloud} busy={sending} uploadingThis={sending} on:close={() => (copiesOpen = false)} on:putBack={(e) => putBack(e.detail)} on:upload={upload} on:deleted={readCloud} />
{/if}

<style>
  .cols {
    display: grid;
    grid-template-columns: minmax(0, 1.6fr) minmax(260px, 1fr);
    gap: 16px;
    align-items: start;
  }
  @media (max-width: 1000px) {
    .cols {
      grid-template-columns: 1fr;
    }
  }
  .card {
    padding: 16px 18px;
    border-radius: var(--radius-lg);
    animation: arrive 0.34s cubic-bezier(0.2, 0.7, 0.2, 1) backwards;
  }
  .cloud {
    animation-delay: 80ms;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  h3 {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: var(--text-base);
    font-weight: 600;
  }
  h3 :global(svg) {
    color: var(--text-faint);
  }
  .rows {
    display: flex;
    flex-direction: column;
    margin-top: 10px;
  }
  .dev {
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 12px 0;
    border-top: 1px solid var(--border);
    animation: arrive 0.32s cubic-bezier(0.2, 0.7, 0.2, 1) backwards;
    animation-delay: calc(var(--i) * 45ms + 40ms);
  }
  .dev:first-child {
    border-top: 0;
    padding-top: 4px;
  }
  .tile {
    width: 40px;
    height: 40px;
    flex: none;
    border-radius: calc(11px * var(--round, 1));
    display: grid;
    place-items: center;
    background: var(--bg-active);
    color: var(--success);
  }
  .here .tile {
    background: rgba(var(--game-accent-rgb, var(--accent-rgb)), 0.18);
    color: var(--game-text, var(--accent-text));
  }
  .offline .tile {
    color: var(--text-faint);
  }
  .text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .name {
    display: flex;
    align-items: center;
    gap: 8px;
    font-weight: 600;
  }
  .this,
  .state {
    font-size: var(--text-xs);
    font-weight: 600;
    color: var(--text-faint);
  }
  .line {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: var(--text-md);
    color: var(--text-dim);
  }
  .line :global(svg) {
    color: var(--text-faint);
  }
  .line.never {
    color: var(--text-faint);
  }
  .line.apart {
    color: var(--warn);
  }
  .line.apart :global(svg) {
    color: var(--warn);
  }
  .path {
    font-size: var(--text-sm);
    color: var(--text-faint);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    direction: rtl;
    text-align: left;
  }
  .none {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 12px 0 2px;
    border-top: 1px solid var(--border);
    color: var(--text-dim);
    font-size: var(--text-md);
  }
  .desc {
    color: var(--text-dim);
    font-size: var(--text-md);
  }
  .cloud-facts {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 8px;
    padding: 10px 12px;
    border-radius: calc(12px * var(--round, 1));
    background: var(--bg-hover);
  }
  .cloud-facts div {
    display: flex;
    flex-direction: column;
  }
  .cap {
    font-size: var(--text-xs);
    color: var(--text-faint);
  }
  .cloud-state {
    font-size: var(--text-md);
    color: var(--text-dim);
  }
  .cloud-state.s-safe {
    color: var(--success);
  }
  .cloud-state.s-behind,
  .cloud-state.s-waiting {
    color: var(--info);
  }
  .cloud-acts {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }
  .arrow {
    display: grid;
    overflow: hidden;
  }
  .sending .arrow :global(svg) {
    animation: send 0.9s cubic-bezier(0.5, 0, 0.5, 1) infinite;
  }
  @keyframes send {
    45% {
      transform: translateY(-16px);
    }
    46% {
      transform: translateY(16px);
    }
  }
  .linked-wrap {
    margin-top: 16px;
    animation: arrive 0.34s cubic-bezier(0.2, 0.7, 0.2, 1) 0.14s backwards;
  }
</style>
