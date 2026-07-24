<script>
	// FRONT-09: a persistent quick-add affordance reachable from anywhere in
	// the app — mounted once in +layout.svelte (not the home route), so it
	// survives navigation and never needs to send the user back to "/" to
	// log a movement. Fully self-sufficient: loads its own
	// categories/currencies/accounts and its own "recent movements" cache
	// rather than depending on state the home route happens to have loaded.
	import { onDestroy, onMount } from 'svelte';
	import { listMovements, createMovement, getCategories, getCurrencies, listAccounts } from '$lib/api.js';
	import { movementCreated, notifyMovementCreated } from '$lib/stores/movementEvents.js';
	import Modal from './Modal.svelte';
	import QuickAddForm from './QuickAddForm.svelte';

	let open = $state(false);
	let categories = $state([]);
	let paymentMethods = $state([]);
	let currencies = $state(['usd', 'brl']);
	let accounts = $state([]);
	let recentMovements = $state([]);
	let recentLoaded = $state(false);

	const mostRecent = $derived(recentMovements[0] ?? null);
	const recentCategories = $derived(dedupeTop(recentMovements.map((m) => m.category), 8));
	const recentDescriptions = $derived(
		dedupeTop(
			recentMovements.map((m) => m.description).filter((d) => d && d.trim()),
			6
		)
	);

	// First occurrence wins, which — since recentMovements is already
	// newest-first from the API — means "most recently used", not just
	// "most frequently used".
	function dedupeTop(values, max) {
		const seen = new Set();
		const out = [];
		for (const v of values) {
			if (!v || seen.has(v)) continue;
			seen.add(v);
			out.push(v);
			if (out.length >= max) break;
		}
		return out;
	}

	async function loadStatic() {
		// allSettled (not Promise.all) so a hiccup on one endpoint doesn't
		// discard the other two's successful responses — same resilience
		// as the home route, which loads each resource independently.
		const [cats, curr, accs] = await Promise.allSettled([
			getCategories(),
			getCurrencies(),
			listAccounts()
		]);
		if (cats.status === 'fulfilled') {
			categories = cats.value.categories ?? [];
			paymentMethods = cats.value.payment_methods ?? [];
		}
		if (curr.status === 'fulfilled' && curr.value.currencies?.length) {
			currencies = curr.value.currencies;
		}
		if (accs.status === 'fulfilled') {
			accounts = accs.value.accounts ?? [];
		}
	}

	async function loadRecent() {
		try {
			// 30 rows is plenty of history to surface 5-8 distinct categories
			// and descriptions without paging; a fresh account just yields [].
			const data = await listMovements({ limit: 30 });
			recentMovements = data.movements ?? [];
		} catch {
			recentMovements = [];
		} finally {
			recentLoaded = true;
		}
	}

	function openQuickAdd() {
		open = true;
	}

	function closeQuickAdd() {
		open = false;
	}

	async function handleFormSubmit(payload) {
		await createMovement(payload);
		// Refreshes this component's own "recent" cache (via the
		// subscription below) and, if the user happens to be on the home
		// route, its movement list too — without either side knowing about
		// the other.
		notifyMovementCreated();
	}

	function isTypingTarget(el) {
		if (!el) return false;
		if (el.isContentEditable) return true;
		return el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.tagName === 'SELECT';
	}

	function handleKeydown(event) {
		if (open) return;
		if (event.ctrlKey || event.metaKey || event.altKey) return;
		if (event.key !== 'n' && event.key !== '/') return;
		if (isTypingTarget(document.activeElement)) return;
		event.preventDefault();
		openQuickAdd();
	}

	// Skip the store's initial callback (fires immediately on subscribe with
	// whatever the current count is) so mount doesn't double-fetch; react
	// only to genuine new movements, from this component or elsewhere.
	let skippedInitialMovementEvent = false;
	const unsubscribeMovementCreated = movementCreated.subscribe(() => {
		if (!skippedInitialMovementEvent) {
			skippedInitialMovementEvent = true;
			return;
		}
		loadRecent();
	});
	onDestroy(unsubscribeMovementCreated);

	onMount(() => {
		loadStatic();
		loadRecent();
	});
</script>

<svelte:window onkeydown={handleKeydown} />

<button
	class="fab"
	type="button"
	onclick={openQuickAdd}
	aria-label="Quick add movement (press n or /)"
	title="Quick add (n or /)"
>
	<span aria-hidden="true">+</span>
</button>

{#if open}
	<Modal title="Quick add" onClose={closeQuickAdd}>
		<QuickAddForm
			{categories}
			{paymentMethods}
			{currencies}
			{accounts}
			{mostRecent}
			{recentCategories}
			{recentDescriptions}
			{recentLoaded}
			onSubmit={handleFormSubmit}
		/>
	</Modal>
{/if}

<style>
	.fab {
		position: fixed;
		right: var(--space-2);
		bottom: var(--space-2);
		width: 3.5rem;
		height: 3.5rem;
		border-radius: 50%;
		background: var(--color-primary);
		color: #fff;
		border: none;
		font-size: 1.8rem;
		line-height: 1;
		display: grid;
		place-items: center;
		box-shadow: var(--shadow-soft);
		z-index: 50;
		transition:
			background var(--transition-fast),
			transform var(--transition-fast);
	}

	.fab:hover {
		background: var(--color-primary-hover);
		transform: scale(1.05);
	}

	.fab:focus-visible {
		outline: none;
		box-shadow: var(--focus-ring);
	}

	@media (min-width: 860px) {
		.fab {
			width: 3rem;
			height: 3rem;
			font-size: 1.5rem;
			right: var(--space-3);
			bottom: var(--space-3);
		}
	}
</style>
