<script lang="ts">
	import * as Tooltip from '#lib/components/ui/tooltip';
	import { helpFor, type HelpKind } from '#lib/help';
	import { Info } from '@lucide/svelte';

	// An (i) that explains a setting. Hover, keyboard focus and tap all open it. Renders
	// nothing when there is no text for the setting, so unknown radio-specific names stay clean.
	let { kind, name, text }: { kind?: HelpKind; name?: string; text?: string } = $props();
	const tip = $derived(text ?? (kind && name ? helpFor(kind, name) : undefined));
</script>

{#if tip}
	<Tooltip.Root delayDuration={150}>
		<Tooltip.Trigger
			type="button"
			class="inline-flex size-4 shrink-0 items-center justify-center rounded-full text-muted-foreground/70 transition-colors hover:text-foreground focus-visible:text-foreground focus-visible:outline-2 focus-visible:outline-ring"
			aria-label="What is this?"
		><Info class="size-3.5" /></Tooltip.Trigger>
		<Tooltip.Content class="max-w-xs text-pretty leading-snug">{tip}</Tooltip.Content>
	</Tooltip.Root>
{/if}
