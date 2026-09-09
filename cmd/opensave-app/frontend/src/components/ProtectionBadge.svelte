<script>
  import {
    protectionState,
    PROTECTION_LABELS,
    PROTECTION_ICONS,
    PROTECTION_DETAILS,
  } from '../lib/protection.js';

  // What protects traffic with one paired device, stated per device.
  //
  // This exists because the honest answer differs per pairing and nobody can
  // work out which case they are in from a general paragraph. Pairing over a
  // relay used to discard the encryption key, so an encrypted pairing and an
  // unencrypted one looked identical in the app — on the page that tells
  // people the relay cannot read their saves. A badge that says "encrypted"
  // when the send path decided otherwise is worse than no badge, so the
  // condition here is the same one the send path applies: a key exists, and
  // the other device has proved it holds the matching half.
  //
  // The fields come from the peers payload; see PeerProtection in
  // internal/p2p/payloadseal.go.
  export let peer;
  /** Compact form for dense lists: icon and label, no explanation. */
  export let compact = false;

  $: state = protectionState(peer);
  $: label = PROTECTION_LABELS[state];
  $: icon = PROTECTION_ICONS[state];
  $: detail = PROTECTION_DETAILS[state];
</script>

<span class="protection {state}" class:compact title={detail}>
  <span class="icon" aria-hidden="true">{icon}</span>
  <span class="label">{label}</span>
</span>

{#if !compact}
  <div class="detail">
    {detail}
    {#if peer?.fingerprint}
      <div class="fingerprint">
        Pairing fingerprint <code>{peer.fingerprint}</code> — it should read the same on the other
        device.
      </div>
    {/if}
  </div>
{/if}

<style>
  .protection {
    display: inline-flex;
    align-items: center;
    gap: 0.35em;
    padding: 0.15em 0.55em;
    border-radius: 999px;
    font-size: 0.78rem;
    font-weight: 600;
    line-height: 1.6;
    white-space: nowrap;
    border: 1px solid transparent;
  }
  .protection.sealed {
    color: var(--ok-fg, #1a7f4b);
    background: var(--ok-bg, rgba(26, 127, 75, 0.12));
    border-color: var(--ok-bg, rgba(26, 127, 75, 0.25));
  }
  .protection.pending {
    color: var(--warn-fg, #8a6100);
    background: var(--warn-bg, rgba(200, 145, 0, 0.12));
    border-color: var(--warn-bg, rgba(200, 145, 0, 0.25));
  }
  /* Deliberately not red. Nothing is broken and no data is at risk of being
     lost — the pairing works exactly as it always did. What is missing is a
     protection that can be switched on, and alarming someone into thinking
     their saves are in danger would misrepresent that. */
  .protection.open {
    color: var(--warn-fg, #8a6100);
    background: var(--warn-bg, rgba(200, 145, 0, 0.12));
    border-color: var(--warn-bg, rgba(200, 145, 0, 0.25));
  }
  .protection.direct {
    color: var(--muted-fg, #5a6270);
    background: var(--muted-bg, rgba(90, 98, 112, 0.1));
    border-color: var(--muted-bg, rgba(90, 98, 112, 0.2));
  }
  .protection.compact {
    font-size: 0.72rem;
    padding: 0.1em 0.45em;
  }
  .protection.compact .label {
    font-weight: 500;
  }
  .detail {
    margin-top: 0.4rem;
    font-size: 0.8rem;
    opacity: 0.75;
    line-height: 1.5;
  }
  .fingerprint {
    margin-top: 0.3rem;
    font-size: 0.76rem;
  }
  .fingerprint code {
    font-size: 0.76rem;
  }
</style>
