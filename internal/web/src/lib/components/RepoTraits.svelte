<script lang="ts">
  // Badges for what the forge says a repository is: a fork, archived, or
  // both.
  import { router } from '../router.svelte';
  import { forgeName, routeForge } from '../forge';
  let { r }: { r: { archived: boolean; fork: boolean } } = $props();
  const forge = $derived(forgeName(routeForge(router.route)));
</script>

{#if r.fork}<span class="badge" title="A fork of another repository">fork</span>{/if}
{#if r.archived}<span class="badge" title={`Archived on ${forge}: read-only`}>archived</span>{/if}
