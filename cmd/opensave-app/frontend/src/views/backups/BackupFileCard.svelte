<script>
  // The backup file: every save and where it belongs, in one .sscb file you
  // can hold. A feature of its own, which people missed while it was a row
  // under the cloud settings, so it is drawn as a cartridge with your covers
  // on its label. Works with or without a cloud.
  import { onMount } from 'svelte';
  import Download from 'lucide-svelte/icons/download';
  import FolderOpen from 'lucide-svelte/icons/folder-open';
  import { api, native, gameCover } from '../../lib/api.js';
  import { gameList, newGames, toast } from '../../lib/stores.js';
  import { t } from '../../lib/i18n.js';
  import CoverImage from '../../components/CoverImage.svelte';
  import ExportDialog from '../cloud/ExportDialog.svelte';
  import ImportDialog from '../cloud/ImportDialog.svelte';

  let last = null; // { madeMs, games }
  async function readLast() {
    try {
      last = await api.get('/api/backup/last-file');
    } catch {
      last = null;
    }
  }
  onMount(readLast);

  $: label = $gameList.slice(0, 4);
  $: madeText = !last?.madeMs
    ? $t('backups.file.never')
    : new Date(last.madeMs).toLocaleDateString(undefined, { day: 'numeric', month: 'short', ...(new Date(last.madeMs).getFullYear() === new Date().getFullYear() ? {} : { year: 'numeric' }) });

  let exporting = false;
  let importSource = '';
  async function open() {
    if (!native.isWails()) {
      toast($t('backups.file.appOnly'));
      return;
    }
    const src = await native.selectBackupFile($t('cloud.backup.selectImportFile'));
    if (src) importSource = src;
  }
</script>

<section class="card file" aria-labelledby="file-title">
  <div class="cart" aria-hidden="true">
    <span class="grip"></span>
    <span class="label">
      {#each label as g (g.id)}<span class="art"><CoverImage src={gameCover(g, true)} alt="" /></span>{:else}<span class="art plain"></span>{/each}
      <span class="label-text"><span class="brand">OPENSAVE</span><span class="count">{$t('backups.file.games', { n: $gameList.length })}</span></span>
    </span>
    <span class="pins"></span>
  </div>
  <div class="text">
    <h2 id="file-title">{$t('backups.file.title')}</h2>
    <p>{$t('backups.file.sub')}</p>
  </div>
  <div class="facts">
    <div><span class="cap">{$t('backups.file.lastMade')}</span><b>{madeText}</b>{#if last?.madeMs}<span class="cap">{$t('backups.file.games', { n: last.games })}</span>{/if}</div>
    <div>
      <span class="cap">{$t('backups.file.canHold')}</span>
      <b>{$newGames.length ? $t('backups.file.trackedFound', { tracked: $gameList.length, found: $newGames.length }) : $t('backups.file.tracked', { n: $gameList.length })}</b>
    </div>
  </div>
  <div class="actions">
    <button class="btn make" on:click={() => (exporting = true)}><Download size={16} />{$t('backups.file.make')}</button>
    <button class="btn" on:click={open}><FolderOpen size={15} />{$t('backups.file.open')}</button>
  </div>
</section>

{#if exporting}
  <ExportDialog
    on:close={() => {
      exporting = false;
      readLast();
    }}
  />
{/if}
{#if importSource}
  <ImportDialog source={importSource} on:close={() => (importSource = '')} />
{/if}

<style>
  .file {
    --gold: #c9a24a;
    --on-gold: #1d1608;
    padding: 24px 20px 20px;
    display: flex;
    flex-direction: column;
    gap: 16px;
    border-radius: var(--radius-lg);
    border: 1px solid var(--border);
    background:
      radial-gradient(120% 70% at 50% 0%, rgba(201, 162, 74, 0.14), rgba(201, 162, 74, 0) 70%),
      var(--bg-raised);
  }
  :global([data-theme='light']) .file {
    --gold: #a27a22;
    --on-gold: #fff;
  }
  /* The cartridge: a shell, a grip, a label of your covers, gold pins. */
  .cart {
    position: relative;
    width: 196px;
    height: 150px;
    margin: 0 auto;
    border-radius: 16px 16px 8px 8px;
    background: linear-gradient(180deg, #34343c, #1f1f25);
    border: 1px solid #45454f;
    box-shadow:
      0 18px 40px rgba(0, 0, 0, 0.5),
      inset 0 1px 0 rgba(255, 255, 255, 0.08);
    transition: transform 0.25s cubic-bezier(0.3, 0.7, 0.3, 1);
  }
  .file:hover .cart {
    transform: translateY(-3px) rotate(-1.5deg);
  }
  .grip {
    position: absolute;
    left: 22px;
    right: 22px;
    top: 9px;
    height: 10px;
    border-radius: 3px;
    background: repeating-linear-gradient(90deg, rgba(255, 255, 255, 0.12) 0 3px, rgba(0, 0, 0, 0) 3px 8px);
  }
  .label {
    position: absolute;
    left: 16px;
    right: 16px;
    top: 26px;
    bottom: 30px;
    border-radius: 8px;
    overflow: hidden;
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    background: #2a2a31;
  }
  .art {
    position: relative;
    overflow: hidden;
  }
  .art.plain {
    grid-column: 1 / -1;
    background: linear-gradient(135deg, #3b3b46, #24242b);
  }
  .art :global(img) {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .label-text {
    position: absolute;
    inset: 0;
    display: flex;
    flex-direction: column;
    justify-content: flex-end;
    padding: 8px 10px;
    color: #fff;
    background: linear-gradient(180deg, rgba(0, 0, 0, 0) 30%, rgba(0, 0, 0, 0.78));
  }
  .brand {
    font-size: 10px;
    font-weight: 700;
    letter-spacing: 0.12em;
    opacity: 0.8;
  }
  .count {
    font-size: var(--text-sm);
    font-weight: 700;
  }
  .pins {
    position: absolute;
    left: 26px;
    right: 26px;
    bottom: 0;
    height: 12px;
    border-radius: 2px 2px 0 0;
    background: repeating-linear-gradient(90deg, #c9a24a 0 6px, rgba(0, 0, 0, 0) 6px 10px);
  }
  .text {
    text-align: center;
  }
  h2 {
    font-size: var(--text-lg);
    font-weight: 600;
  }
  .text p {
    margin-top: 6px;
    font-size: var(--text-md);
    color: var(--text-dim);
  }
  .facts {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 8px;
    padding: 10px 12px;
    border-radius: 12px;
    background: var(--bg-hover);
    font-size: var(--text-md);
  }
  .facts div {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .facts b {
    font-weight: 600;
  }
  .cap {
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
  .actions {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .make {
    height: 40px;
    background: var(--gold);
    border-color: var(--gold);
    color: var(--on-gold);
    font-weight: 600;
  }
  .make:hover {
    filter: brightness(1.08);
  }
</style>
