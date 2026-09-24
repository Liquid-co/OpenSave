<!-- The bottom bar of a Modal: a note or secondary actions on the left, the
     main actions on the right. -->
<div class="foot"><slot /></div>

<style>
  .foot {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 12px;
    padding: 14px 22px;
    border-top: 1px solid var(--border);
  }
</style>
