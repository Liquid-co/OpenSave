<script>
  // A new branch: its name, and what it starts from. The two answers do
  // different things to the save folder on the next switch, so they are two
  // choices to read, not a tickbox. Fires `create` with { name, copy } (copy:
  // a copy of the current save, or empty), `cancel` otherwise.
  import { createEventDispatcher, onMount } from 'svelte';
  import { t } from '../../lib/i18n.js';
  import Modal from '../../components/ui/Modal.svelte';
  import ModalFoot from '../../components/ui/ModalFoot.svelte';
  import GitBranch from 'lucide-svelte/icons/git-branch';
  import Copy from 'lucide-svelte/icons/copy';
  import Sparkles from 'lucide-svelte/icons/sparkles';
  import History from 'lucide-svelte/icons/history';

  export let activeBranch = '';
  export let taken = [];
  export let busy = false;

  const dispatch = createEventDispatcher();
  let name = '';
  // Default to copying: a branch that keeps your save can never surprise you.
  let copy = true;
  let field;
  onMount(() => field?.focus());

  // As the daemon cleans it (snapshot.CleanBranchName): lower case, letters,
  // digits, - and _.
  $: clean = name.toLowerCase().replace(/[^a-z0-9_-]/g, '');
  $: problem = !name.trim() ? '' : !clean ? $t('game.newBranch.badName') : taken.includes(clean) ? $t('game.newBranch.taken', { name: clean }) : '';
  const create = () => clean && !problem && !busy && dispatch('create', { name: clean, copy });
</script>

<Modal title={$t('game.newBranch.dialogTitle')} icon={GitBranch} width={560} height="auto" onClose={() => dispatch('cancel')}>
  <svelte:fragment slot="sub">{$t('game.newBranch.sub')}</svelte:fragment>
  <div class="body">
    <label class="field">
      <span>{$t('game.newBranch.nameLabel')}</span>
      <input bind:this={field} bind:value={name} placeholder={$t('game.branches.namePlaceholder')} maxlength="60" on:keydown={(e) => e.key === 'Enter' && create()} />
      {#if problem}<span class="problem" role="alert">{problem}</span>{:else if clean && clean !== name}<span class="hint">{$t('game.newBranch.cleaned', { name: clean })}</span>{/if}
    </label>

    <div class="label" id="nb-start">{$t('game.newBranch.question')}</div>
    <div class="choices" role="radiogroup" aria-labelledby="nb-start">
      <button class="choice" class:sel={copy} role="radio" aria-checked={copy} on:click={() => (copy = true)}>
        <span class="ctile"><Copy size={17} /></span>
        <span class="ctext"><span class="ctitle">{$t('game.newBranch.copyTitle')}<span class="tag">{$t('cloud.import.recommended')}</span></span><span class="cdesc">{$t('game.newBranch.copyDesc', { name: clean || '…' })}</span></span>
        <span class="radio" aria-hidden="true"></span>
      </button>
      <button class="choice" class:sel={!copy} role="radio" aria-checked={!copy} on:click={() => (copy = false)}>
        <span class="ctile"><Sparkles size={17} /></span>
        <span class="ctext"><span class="ctitle">{$t('game.newBranch.freshTitle')}</span><span class="cdesc">{$t('game.newBranch.freshDesc1', { name: clean || '…' })}<strong>{activeBranch}</strong>{$t('game.newBranch.freshDesc2')}</span></span>
        <span class="radio" aria-hidden="true"></span>
      </button>
    </div>
  </div>
  <ModalFoot>
    <span class="note"><History size={15} />{$t('game.newBranch.hint')}</span>
    <div class="actions">
      <button class="btn" on:click={() => dispatch('cancel')}>{$t('common.cancel')}</button>
      <button class="btn primary" disabled={busy || !clean || !!problem} on:click={create}>{$t('game.branches.create')}</button>
    </div>
  </ModalFoot>
</Modal>

<style>
  .body {
    padding: 18px 22px;
    display: flex;
    flex-direction: column;
    gap: 14px;
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: 6px;
    font-size: var(--text-md);
    font-weight: 600;
    color: var(--text-dim);
  }
  .problem {
    font-weight: 400;
    color: var(--danger-text);
  }
  .hint {
    font-weight: 400;
    color: var(--text-faint);
  }
  .label {
    font-size: var(--text-md);
    font-weight: 600;
    color: var(--text-dim);
  }
  .choices {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .choice {
    display: flex;
    align-items: flex-start;
    gap: 14px;
    padding: 14px 16px 14px 14px;
    border-radius: calc(14px * var(--round, 1));
    border: 1.5px solid var(--border);
    background: var(--bg);
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
    transition:
      border-color 0.18s,
      background 0.18s,
      transform 0.18s cubic-bezier(0.2, 0.7, 0.2, 1);
  }
  .choice:hover {
    border-color: var(--border-strong);
    transform: translateY(-1px);
  }
  .choice.sel {
    border-color: var(--accent);
    background: linear-gradient(0deg, var(--accent-soft), var(--accent-soft)), var(--bg);
  }
  .ctile {
    width: 36px;
    height: 36px;
    flex: none;
    border-radius: calc(10px * var(--round, 1));
    display: grid;
    place-items: center;
    background: var(--bg-active);
    color: var(--text-dim);
    transition:
      background 0.18s,
      color 0.18s;
  }
  .sel .ctile {
    background: rgba(var(--accent-rgb), 0.22);
    color: var(--accent-text);
  }
  .ctext {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .ctitle {
    display: flex;
    align-items: center;
    gap: 8px;
    font-weight: 600;
  }
  .tag {
    padding: 1px 8px;
    border-radius: 999px;
    font-size: var(--text-xs);
    font-weight: 600;
    color: var(--success);
    background: rgba(var(--success-rgb), 0.14);
  }
  .cdesc {
    font-size: var(--text-md);
    color: var(--text-dim);
    line-height: 1.45;
  }
  .radio {
    position: relative;
    width: 18px;
    height: 18px;
    flex: none;
    margin-top: 2px;
    border-radius: 50%;
    border: 2px solid var(--border-strong);
    transition: border-color 0.18s;
  }
  .radio::after {
    content: '';
    position: absolute;
    inset: 3px;
    border-radius: 50%;
    background: var(--accent);
    transform: scale(0);
    transition: transform 0.2s cubic-bezier(0.3, 1.4, 0.5, 1);
  }
  .sel .radio {
    border-color: var(--accent);
  }
  .sel .radio::after {
    transform: scale(1);
  }
  .note {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    font-size: var(--text-sm);
    color: var(--text-faint);
    line-height: 1.4;
  }
  .note :global(svg) {
    flex: none;
  }
  .actions {
    display: flex;
    gap: 8px;
  }
</style>
