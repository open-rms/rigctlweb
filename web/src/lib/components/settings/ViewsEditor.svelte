<script lang="ts">
	import { app } from '#lib/stores/rig.svelte';
	import { views } from '#lib/stores/views.svelte';
	import { catalog, GROUPS, BUILTIN_IDS, type Control } from '#lib/views';
	import { Button } from '#lib/components/ui/button';
	import { Input } from '#lib/components/ui/input';
	import NativeSelect from '#lib/components/ui/native-select.svelte';
	import Help from '#lib/components/ui/help.svelte';
	import { toast } from 'svelte-sonner';
	import { Copy, Pencil, RotateCcw, Trash2, Check } from '@lucide/svelte';

	let editingId = $state<string>(views.activeId);
	let query = $state('');
	let newName = $state('');
	let newFrom = $state('');
	let renaming = $state<string | null>(null);
	let renameText = $state('');

	const list = $derived(views.all);
	const editing = $derived(views.get(editingId) ?? list[0]);
	const cat = $derived(catalog(app.caps));
	const catIds = $derived(new Set(cat.map((c) => c.id)));
	const readOnlyView = $derived(!!editing?.all);

	const filtered = $derived.by(() => {
		const q = query.trim().toLowerCase();
		return cat.filter((c) => !q || c.label.toLowerCase().includes(q) || c.id.toLowerCase().includes(q));
	});
	const byGroup = $derived(
		GROUPS.map((g) => ({ group: g, items: filtered.filter((c) => c.group === g) })).filter((g) => g.items.length)
	);

	const on = (id: string) => !!editing && (editing.all || editing.controls.includes(id));
	const unsupported = $derived(editing && !editing.all ? editing.controls.filter((id) => !catIds.has(id)).length : 0);
	const countOn = $derived(cat.filter((c) => on(c.id)).length);

	function toggle(c: Control, checked: boolean) {
		if (editing) views.setControls(editing.id, [c.id], checked);
	}
	function setGroup(items: Control[], checked: boolean) {
		if (editing) views.setControls(editing.id, items.map((i) => i.id), checked);
	}
	function create(e: SubmitEvent) {
		e.preventDefault();
		if (!newName.trim()) return;
		const id = views.create(newName, newFrom || undefined);
		editingId = id;
		newName = '';
		newFrom = '';
		toast.success('View created and selected');
	}
	function startRename(id: string, name: string) {
		renaming = id;
		renameText = name;
	}
	function finishRename() {
		if (renaming) views.rename(renaming, renameText);
		renaming = null;
	}
	function duplicate(id: string) {
		const src = views.get(id);
		if (!src) return;
		editingId = views.create(`${src.name} copy`, id);
	}
	function remove(id: string, name: string) {
		if (!confirm(`Delete the view "${name}"?`)) return;
		views.remove(id);
		if (editingId === id) editingId = views.activeId;
	}
</script>

<div class="space-y-5">
	<p class="text-sm text-muted-foreground">
		A view chooses which controls you see. Pick one from the menu at the top of the page. Edit the built-in views to suit you
		(you can always reset them), or make your own, optionally starting from a copy of another view.
		<Help kind="ui" name="views" />
	</p>

	<ul class="divide-y rounded-lg border">
		{#each list as v (v.id)}
			<li class="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2.5 {editingId === v.id ? 'bg-accent/50' : ''}">
				<button type="button" class="flex min-w-0 flex-1 items-start gap-3 text-left" onclick={() => (editingId = v.id)} aria-pressed={editingId === v.id}>
					<span class="mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full border {views.activeId === v.id ? 'border-primary bg-primary text-primary-foreground' : ''}" title={views.activeId === v.id ? 'Active view' : ''}>
						{#if views.activeId === v.id}<Check class="size-3" />{/if}
					</span>
					<span class="min-w-0">
						{#if renaming === v.id}
							<!-- svelte-ignore a11y_autofocus -->
							<input class="h-7 rounded border bg-background px-2 text-sm" bind:value={renameText} maxlength="40" autofocus
								onclick={(e) => e.stopPropagation()} onblur={finishRename}
								onkeydown={(e) => { if (e.key === 'Enter') finishRename(); else if (e.key === 'Escape') renaming = null; }} />
						{:else}
							<span class="block truncate text-sm font-medium">
								{v.name}
								{#if BUILTIN_IDS.has(v.id)}<span class="ml-1.5 rounded bg-secondary px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-secondary-foreground">Built-in{v.modified ? ', edited' : ''}</span>{/if}
							</span>
						{/if}
						{#if v.description}<span class="block text-xs text-muted-foreground">{v.description}</span>{/if}
					</span>
				</button>
				<div class="flex items-center gap-1">
					{#if views.activeId !== v.id}<Button size="xs" variant="outline" onclick={() => views.setActive(v.id)}>Use</Button>{/if}
					<Button size="icon-xs" variant="ghost" onclick={() => duplicate(v.id)} aria-label="Duplicate {v.name}" title="Make a copy to edit"><Copy /></Button>
					{#if !BUILTIN_IDS.has(v.id)}
						<Button size="icon-xs" variant="ghost" onclick={() => startRename(v.id, v.name)} aria-label="Rename {v.name}" title="Rename"><Pencil /></Button>
						<Button size="icon-xs" variant="ghost" onclick={() => remove(v.id, v.name)} aria-label="Delete {v.name}" title="Delete"><Trash2 /></Button>
					{:else if v.modified}
						<Button size="icon-xs" variant="ghost" onclick={() => { views.reset(v.id); toast.success(`${v.name} restored to its defaults`); }} aria-label="Reset {v.name} to default" title="Reset to default"><RotateCcw /></Button>
					{/if}
				</div>
			</li>
		{/each}
	</ul>

	<form onsubmit={create} class="grid gap-3 rounded-lg border p-3 sm:grid-cols-[1fr_1fr_auto]">
		<label class="space-y-1 text-sm">New view name<Input bind:value={newName} placeholder="My field day" maxlength={40} required /></label>
		<label class="space-y-1 text-sm">Start from
			<NativeSelect bind:value={newFrom}>
				<option value="">Blank</option>
				{#each list as v}<option value={v.id}>{v.name}</option>{/each}
			</NativeSelect>
		</label>
		<Button type="submit" class="self-end">Create</Button>
	</form>

	{#if editing}
		<div class="space-y-3 rounded-lg border p-3">
			<div class="flex flex-wrap items-center justify-between gap-2">
				<div>
					<div class="text-sm font-medium">Editing: {editing.name}</div>
					<div class="text-xs text-muted-foreground">
						{#if readOnlyView}Shows every control your radio offers. Duplicate it to make a trimmed copy.
						{:else if !app.caps}Connect a radio to choose controls.
						{:else}{countOn} of {cat.length} controls on for {app.caps.mfg} {app.caps.model}{unsupported ? `; ${unsupported} saved control${unsupported === 1 ? '' : 's'} not available on this radio are kept` : ''}.{/if}
					</div>
				</div>
				{#if views.saving}<span class="text-xs text-muted-foreground">Saving…</span>{:else if views.error}<span class="text-xs text-destructive">{views.error}</span>{/if}
			</div>

			{#if !readOnlyView && app.caps}
				<Input bind:value={query} placeholder="Search controls" aria-label="Search controls" />
				<div class="space-y-4">
					{#each byGroup as g (g.group)}
						<fieldset class="space-y-1.5">
							<legend class="flex w-full items-center justify-between text-xs font-semibold uppercase tracking-wide text-muted-foreground">
								<span>{g.group}</span>
								<span class="space-x-2 font-normal normal-case tracking-normal">
									<button type="button" class="underline hover:text-foreground" onclick={() => setGroup(g.items, true)}>all</button>
									<button type="button" class="underline hover:text-foreground" onclick={() => setGroup(g.items, false)}>none</button>
								</span>
							</legend>
							<div class="grid gap-x-4 gap-y-1 sm:grid-cols-2">
								{#each g.items as c (c.id)}
									<label class="flex min-w-0 items-center gap-2 py-0.5 text-sm">
										<input type="checkbox" class="size-4 accent-[var(--primary)]" checked={on(c.id)} onchange={(e) => toggle(c, e.currentTarget.checked)} />
										<span class="truncate">{c.label}</span>
									</label>
								{/each}
							</div>
						</fieldset>
					{/each}
					{#if !byGroup.length}<p class="text-sm text-muted-foreground">No controls match "{query}".</p>{/if}
				</div>
			{/if}
		</div>
	{/if}
</div>
