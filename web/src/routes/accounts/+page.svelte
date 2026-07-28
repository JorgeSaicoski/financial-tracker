<script>
	import { onMount } from 'svelte';
	import { listAccounts, reportAccountBalance } from '$lib/api.js';
	import AccountHistoryCard from '$lib/components/AccountHistoryCard.svelte';

	let accounts = $state([]);
	let loading = $state(true);
	let error = $state('');

	async function load() {
		error = '';
		try {
			const data = await listAccounts();
			accounts = data.accounts ?? [];
		} catch (err) {
			error = err.message;
		} finally {
			loading = false;
		}
	}

	async function handleReportBalance(accountId, cents, timestamp) {
		const updated = await reportAccountBalance(accountId, cents, timestamp);
		accounts = accounts.map((a) => (a.id === updated.id ? updated : a));
	}

	onMount(load);
</script>

<main>
	<header>
		<h1>Account history</h1>
		<a class="ghost" href="/">← Back</a>
	</header>

	{#if error}
		<p class="message error">{error}</p>
	{/if}

	{#if loading}
		<p class="empty">Loading…</p>
	{:else if accounts.length === 0}
		<p class="empty">No accounts yet — add one from the dashboard first.</p>
	{:else}
		{#each accounts as account (account.id)}
			<AccountHistoryCard {account} onReportBalance={handleReportBalance} />
		{/each}
	{/if}
</main>

<style>
	main {
		max-width: 560px;
		margin: 0 auto;
		padding: var(--space-4) var(--space-2) var(--space-6);
		font: var(--text-body);
		color: var(--color-text-primary);
	}

	header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: var(--space-3);
	}

	h1 {
		font: var(--text-page-title);
		margin: 0;
		color: var(--color-primary);
		letter-spacing: -0.02em;
	}

	.empty {
		text-align: center;
		color: var(--color-text-secondary);
		padding: var(--space-3) 0;
	}
</style>
