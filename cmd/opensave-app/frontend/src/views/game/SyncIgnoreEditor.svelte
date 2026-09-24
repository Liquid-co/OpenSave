<script>
  // Files that shouldn't sync: a pattern box, and a list of what is actually
  // in the game's folders to tick instead.
  //
  // The pattern box on its own asks you to type a filename you have to
  // already know, for a folder you cannot see, in a syntax you have to learn
  // — and gives no answer until the file turns up on another machine days
  // later. The list marks each file with the verdict the sync engine itself
  // would reach, so the rule and its effect are on screen together.
  import { onDestroy } from 'svelte';
  import { api } from '../../lib/api.js';
  import { addExclusion, addNegation, removeDirectExclusion } from '../../lib/ignorerules.js';
  import Spinner from '../../components/ui/Spinner.svelte';
  import Chevron from '../../components/ui/Chevron.svelte';

  export let game;
  /** The rule text being edited; saved with the rest of the form. */
  export let value = '';
  /** Whether the game has folders besides its save folder. */
  export let severalFolders = false;

  let showFiles = false;
  let saveFiles = null;
  let truncated = false;
  let filesError = '';
  $: excludedCount = (saveFiles ?? []).filter((f) => f.excluded).length;

  // Ask the daemon to judge a rule list against the real folders. The rules
  // are sent rather than saved first: the point of a verdict is watching it
  // change as you type, and saving to find out would mean writing a rule to
  // discover whether it was the rule you meant.
  async function refresh(rulesText) {
    try {
      const q = encodeURIComponent(rulesText ?? '');
      const res = await api.get(`/api/games/${game.id}/save-files?rules=${q}`);
      saveFiles = res?.files ?? [];
      truncated = !!res?.truncated;
      filesError = '';
    } catch (e) {
      filesError = e.message;
    }
  }

  async function togglePicker() {
    showFiles = !showFiles;
    if (showFiles && saveFiles === null) await refresh(value ?? '');
  }

  // Typing in the box re-judges the list, but not on every keystroke.
  let previewTimer = null;
  let lastPreviewed = null;
  onDestroy(() => clearTimeout(previewTimer));
  $: if (showFiles && value !== lastPreviewed) {
    lastPreviewed = value;
    clearTimeout(previewTimer);
    const text = value ?? '';
    previewTimer = setTimeout(() => refresh(text), 250);
  }

  // Ticking a file adds its pattern. Unticking removes the lines that name it
  // outright — and if it is still caught after that, by a wildcard or a folder
  // rule, adds a "!" exception, which is the only thing that can rescue one
  // file from a broader rule without abandoning the rule.
  //
  // Whether it is still caught is a question only the daemon can answer: it
  // holds the matcher the sync engine itself uses. The text editing around
  // that answer lives in ../../lib/ignorerules.js, where it is tested.
  async function toggleFile(f) {
    let text;
    if (!f.excluded) {
      text = addExclusion(value, f.path);
    } else {
      text = removeDirectExclusion(value, f.path);
      await refresh(text);
      const still = (saveFiles ?? []).find((x) => x.path === f.path && x.location === f.location);
      if (still?.excluded) text = addNegation(text, f.path);
    }
    value = text;
    lastPreviewed = text;
    await refresh(text);
  }
</script>

<div class="section">
  <h4>Files that shouldn't sync</h4>
  <p class="hint">
    Some games keep device-specific settings in the same folder as the save, and copying
    those to another machine can break the game there. List them here and they stay put:
    never sent, never received, never deleted by a sync.
  </p>
  <textarea
    class="box"
    rows="4"
    spellcheck="false"
    placeholder={'Config.gs\n*.log\nlogs/'}
    bind:value
  ></textarea>
  <span class="hint">
    One pattern per line, like a <code>.gitignore</code> — <code>Config.gs</code> by name,
    <code>/Config.gs</code> only at the top, <code>*.log</code> by extension,
    <code>logs/</code> for a whole folder, <code>!keep.log</code> for an exception. Case
    doesn't matter. Set the same list on your other devices; each one applies its own.
  </span>
  <span class="hint">
    <strong>Snapshots still capture these files</strong>, so a restore brings them back —
    excluding something stops it travelling, it never stops it being backed up.
  </span>

  <div class="picker-head">
    <button class="btn small" on:click={togglePicker}>
      <Chevron open={showFiles} /> Pick from your save folder
    </button>
    {#if showFiles && saveFiles}
      <span class="hint picker-count">
        {excludedCount} of {saveFiles.length} files excluded
        {#if truncated}· first {saveFiles.length} shown{/if}
      </span>
    {/if}
  </div>

  {#if showFiles}
    {#if filesError}
      <p class="hint err">{filesError}</p>
    {:else if !saveFiles}
      <p class="hint"><Spinner size={13} /> Reading the save folder…</p>
    {:else if saveFiles.length === 0}
      <p class="hint">This game's folders are empty, so there is nothing to pick yet.</p>
    {:else}
      <div class="picker">
        {#each saveFiles as f (f.location + '/' + f.path)}
          <label class="file-row" class:excluded={f.excluded}>
            <input type="checkbox" checked={f.excluded} on:change={() => toggleFile(f)} />
            <span class="file-name">
              {#if f.location}<span class="file-loc">{f.location} ›</span>{/if}{f.path}
            </span>
            <span class="verdict">{f.excluded ? "won't sync" : 'syncs'}</span>
          </label>
        {/each}
      </div>
      <span class="hint">
        Ticking a file writes the pattern for you, anchored so it can only ever mean that
        one file. Unticking a file caught by a wildcard adds a <code>!</code> exception
        rather than deleting the wildcard.
        {#if severalFolders}
          A pattern applies to <strong>every one of this game's folders</strong>, so a
          name that appears in two of them is excluded in both.
        {/if}
      </span>
    {/if}
  {/if}
</div>

<style>
  .section {
    margin-top: 18px;
    padding-top: 16px;
    border-top: 1px solid var(--border);
  }
  h4 {
    font-size: 0.92rem;
    font-weight: 600;
    margin-bottom: 6px;
  }
  .box {
    width: 100%;
    margin-top: 8px;
    padding: 10px 12px;
    background: var(--bg);
    border: 1px solid var(--border-strong);
    border-radius: 8px;
    color: var(--text);
    font-family: ui-monospace, 'Cascadia Code', Consolas, monospace;
    font-size: 0.82rem;
    line-height: 1.6;
    resize: vertical;
    outline: none;
  }
  .box:focus {
    border-color: var(--accent);
  }
  .hint + .hint {
    margin-top: 6px;
  }
  .picker-head {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-top: 12px;
    flex-wrap: wrap;
  }
  .picker-count {
    margin-top: 0;
  }
  .picker {
    margin-top: 8px;
    max-height: 280px;
    overflow-y: auto;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--bg);
  }
  .file-row {
    display: flex;
    align-items: center;
    gap: 9px;
    padding: 5px 10px;
    border-bottom: 1px solid var(--border);
    cursor: pointer;
    font-size: 0.8rem;
  }
  .file-row:last-child {
    border-bottom: 0;
  }
  .file-row:hover {
    background: var(--bg-elev, rgba(127, 127, 127, 0.06));
  }
  .file-name {
    flex: 1;
    min-width: 0;
    font-family: ui-monospace, 'Cascadia Code', Consolas, monospace;
    word-break: break-all;
  }
  /* The location a file lives in, when the game has more than its save
     folder — the same filename can appear in two of them. */
  .file-loc {
    color: var(--accent);
    margin-right: 6px;
  }
  .verdict {
    flex: 0 0 auto;
    font-size: 0.72rem;
    color: var(--text-faint);
  }
  .file-row.excluded .file-name {
    text-decoration: line-through;
    color: var(--text-faint);
  }
  .file-row.excluded .verdict {
    color: var(--warn, var(--accent));
  }
  .err {
    color: var(--danger, var(--danger));
  }
</style>
