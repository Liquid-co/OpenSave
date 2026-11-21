<!-- The bottom bar of a Modal: a note or secondary actions on the left, the
     main actions on the right. A shade darker than the dialog, as the sync
     panel's actions are, so what you do is set apart from what you read. -->
<div class="foot"><slot /></div>

<style>
  .foot {
    flex-shrink: 0;
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 12px;
    padding: 14px 20px 14px 22px;
    border-top: 1px solid var(--border);
    background: var(--bg-sidebar);
    border-radius: 0 0 var(--modal-r, calc(14px * var(--round, 1))) var(--modal-r, calc(14px * var(--round, 1)));
  }
</style>
