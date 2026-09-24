<script>
  // A provider's logo, or a plain glyph for the ones without a brand.
  export let provider;
  export let size = 34;

  const paths = {
    folder: 'M20 6h-8l-2-2H4c-1.1 0-1.99.9-1.99 2L2 18c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V8c0-1.1-.9-2-2-2z',
    cloud: 'M19.35 10.04C18.67 6.59 15.64 4 12 4 9.11 4 6.6 5.64 5.35 8.04 2.34 8.36 0 10.91 0 14c0 3.31 2.69 6 6 6h13c2.76 0 5-2.24 5-5 0-2.64-2.05-4.78-4.65-4.96z',
    webhook: 'M20 4H4c-1.1 0-1.99.9-1.99 2L2 18c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V6c0-1.1-.9-2-2-2zm0 14H4V8h16v10zM12 10H8v2h4v-2zm4 4h-8v2h8v-2z'
  };
</script>

{#if provider?.img}
  <img src={provider.img} alt={provider.label} style="width: {size}px; height: {size}px" />
{:else}
  <svg viewBox="0 0 24 24" width={size} height={size} fill="currentColor"><path d={paths[provider?.icon ?? 'cloud']} /></svg>
{/if}

<style>
  img {
    object-fit: contain;
  }
</style>
