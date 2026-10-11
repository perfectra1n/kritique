<script lang="ts">
  // Whether the forge's webhooks reach kritique for a connection: receiving,
  // arriving unsigned, which kritique refuses, or none, so that it only
  // polls. With polled, when it last polled is said while they do not arrive.
  import { unsignedWebhooks } from '../setup';
  import { forgeName } from '../forge';
  import type { Connection } from '../types';
  import Pill from './Pill.svelte';
  import Time from './Time.svelte';

  let {
    of,
    polled,
  }: { of: Pick<Connection, 'lastWebhookAt' | 'lastUnsignedWebhookAt'> & { forge?: string }; polled?: { at: string | null } } = $props();
  const unsigned = $derived(unsignedWebhooks(of));
  const unsignedWhy = $derived(
    of.forge === 'github'
      ? "GitHub sends the App's webhooks with no signature, so kritique refuses them and only polls: set the App's webhook secret"
      : `The webhooks arrive with no signature, so kritique refuses them and only polls: set the webhook's secret on ${forgeName(of.forge)}`,
  );
</script>

{#if unsigned}
  <Pill tone="danger" label="unsigned" title={unsignedWhy} />
{:else if of.lastWebhookAt}
  <Pill tone="ok" label="receiving" />
{:else}
  <Pill tone="warn" label="polling only" title="No webhook has reached kritique, so it only polls for new pull requests and cannot answer mentions" />
{/if}
<span class="small muted">
  {#if polled && (unsigned || !of.lastWebhookAt)}
    {#if polled.at}polled <Time iso={polled.at} />{:else}not polled yet{/if}
  {:else if unsigned}
    <Time iso={of.lastUnsignedWebhookAt} />
  {:else if of.lastWebhookAt}
    <Time iso={of.lastWebhookAt} />
  {/if}
</span>
