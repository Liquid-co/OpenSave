<script>
  // What a new branch starts from. Fires `create` with true for a copy of the
  // current save, false for an empty one; `cancel` otherwise.
  import { createEventDispatcher } from 'svelte';
  import GitBranch from 'lucide-svelte/icons/git-branch';
  import ShieldCheck from 'lucide-svelte/icons/shield-check';

  export let name = '';
  export let activeBranch = '';
  export let busy = false;

  const dispatch = createEventDispatcher();
  // Default to copying: a branch that keeps your save can never surprise you.
  let copySave = true;

  // Escape closes it, as in every other dialog in the app.
  const onKeydown = (e) => e.key === 'Escape' && dispatch('cancel');
</script>

<svelte:window on:keydown={onKeydown} />

<!-- The same furniture as the conflict modal on purpose — both are "this is
     about to touch your save folder, choose". -->
<!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
<div class="overlay" on:click={() => dispatch('cancel')}>
  <!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
  <div class="modal card" on:click|stopPropagation>
    <h3 class="with-icon"><GitBranch size={19} /> New branch — {name}</h3>
    <p class="desc">What should it start from?</p>

    <label class="choice" class:sel={copySave}>
      <input type="radio" bind:group={copySave} value={true} />
      <span class="c-body">
        <span class="c-title">A copy of my current save</span>
        <span class="c-desc">
          "{name}" begins exactly where you are now. The two only diverge from the first
          snapshot you take on it — so this is the one for trying something without losing your
          place.
        </span>
      </span>
    </label>

    <label class="choice" class:sel={!copySave}>
      <input type="radio" bind:group={copySave} value={false} />
      <span class="c-body">
        <span class="c-title">A fresh start — no save</span>
        <span class="c-desc">
          "{name}" begins empty, for a new playthrough from scratch. Switching to it clears
          your save folder — your current save is snapshotted first and comes straight back when
          you switch to <strong>{activeBranch}</strong>.
        </span>
      </span>
    </label>

    <div class="actions">
      <button class="btn" on:click={() => dispatch('cancel')}>Cancel</button>
      <button class="btn primary" disabled={busy} on:click={() => dispatch('create', copySave)}>Create branch</button>
    </div>
    <p class="hint-line">
      <ShieldCheck size={15} class="inline-icon" /> Either way nothing is lost. Switching between branches snapshots whatever is in your
      save folder first, and refuses to change anything if that snapshot can't be taken.
    </p>
  </div>
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: var(--overlay);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 90;
  }
  .modal {
    width: 520px;
    max-width: calc(100vw - 48px);
    max-height: calc(100vh - 80px);
    overflow-y: auto;
  }
  .modal h3 {
    margin-bottom: 8px;
    overflow-wrap: anywhere;
  }
  .desc {
    color: var(--text-dim);
    font-size: 0.9rem;
    margin-bottom: 14px;
  }
  .choice {
    display: flex;
    align-items: flex-start;
    gap: 11px;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 12px;
    margin-bottom: 10px;
    cursor: pointer;
    transition:
      border-color 0.13s ease,
      background 0.13s ease;
  }
  .choice:hover {
    border-color: var(--border-strong);
  }
  .choice.sel {
    border-color: var(--accent);
    background: var(--accent-soft);
  }
  .choice input {
    margin-top: 1px;
  }
  .c-body {
    display: flex;
    flex-direction: column;
    gap: 4px;
    min-width: 0;
  }
  .c-title {
    font-size: 0.9rem;
    font-weight: 600;
  }
  .c-desc {
    font-size: 0.8rem;
    color: var(--text-dim);
    line-height: 1.5;
  }
  .actions {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
    margin-top: 14px;
  }
  .hint-line {
    margin-top: 12px;
    font-size: 0.78rem;
    color: var(--text-faint);
    line-height: 1.5;
  }
</style>
