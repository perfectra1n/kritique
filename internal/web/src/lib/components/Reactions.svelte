<script lang="ts">
  // The 👍 and 👎 a finding's inline comment drew on the forge; nothing
  // when it drew none.
  import Icon from '../Icon.svelte';
  import { mdiThumbUpOutline, mdiThumbDownOutline } from '../icons';
  import { router } from '../router.svelte';
  import { forgeName, routeForge } from '../forge';
  let { up, down }: { up: number; down: number } = $props();
  const forge = $derived(forgeName(routeForge(router.route)));
</script>

{#if up || down}
  <span class="reactions" title={`Reactions to its comment on ${forge}`}>
    {#if up}<span><Icon path={mdiThumbUpOutline} size={12} label="thumbs up" />{up}</span>{/if}
    {#if down}<span><Icon path={mdiThumbDownOutline} size={12} label="thumbs down" />{down}</span>{/if}
  </span>
{/if}
