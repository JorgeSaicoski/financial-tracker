<script>
	import { onMount } from 'svelte';
	import { listMovements, createMovement } from '$lib/api.js';

	let movements = $state([]);
	let balance = $state(0);
	let loading = $state(true);
	let error = $state('');

	let amountInput = $state('');
	let directionInput = $state('expense');
	let currencyInput = $state('usd');
	let submitting = $state(false);

	function formatAmount(cents, currency) {
		return new Intl.NumberFormat('en-US', {
			style: 'currency',
			currency: currency.toUpperCase()
		}).format(cents / 100);
	}

	async function load() {
		loading = true;
		error = '';
		try {
			const data = await listMovements();
			movements = data.movements ?? [];
			balance = data.balance ?? 0;
		} catch (err) {
			error = err.message;
		} finally {
			loading = false;
		}
	}

	async function handleSubmit(event) {
		event.preventDefault();

		const cents = Math.round(parseFloat(amountInput) * 100);
		if (!cents) {
			error = 'Enter a non-zero amount';
			return;
		}
		const signedAmount = directionInput === 'expense' ? -Math.abs(cents) : Math.abs(cents);

		submitting = true;
		error = '';
		try {
			await createMovement({ amount: signedAmount, currency: currencyInput });
			amountInput = '';
			await load();
		} catch (err) {
			error = err.message;
		} finally {
			submitting = false;
		}
	}

	onMount(load);
</script>

<main>
	<h1>Financial Tracker</h1>

	<section class="balance">
		<span>Balance</span>
		<strong>{formatAmount(balance, currencyInput)}</strong>
	</section>

	<form onsubmit={handleSubmit}>
		<input type="number" step="0.01" placeholder="Amount" bind:value={amountInput} required />
		<select bind:value={directionInput}>
			<option value="expense">Expense</option>
			<option value="income">Income</option>
		</select>
		<select bind:value={currencyInput}>
			<option value="usd">USD</option>
			<option value="brl">BRL</option>
		</select>
		<button type="submit" disabled={submitting}>Add movement</button>
	</form>

	{#if error}
		<p class="error">{error}</p>
	{/if}

	{#if loading}
		<p>Loading…</p>
	{:else if movements.length === 0}
		<p>No movements yet.</p>
	{:else}
		<ul class="movements">
			{#each movements as movement (movement.id)}
				<li class:credit={movement.amount > 0} class:debit={movement.amount < 0}>
					<span>{new Date(movement.timestamp).toLocaleString()}</span>
					<span>{formatAmount(movement.amount, movement.currency)}</span>
				</li>
			{/each}
		</ul>
	{/if}
</main>

<style>
	main {
		max-width: 480px;
		margin: 2rem auto;
		font-family: system-ui, sans-serif;
		padding: 0 1rem;
	}

	.balance {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
		padding: 1rem;
		border: 1px solid #ddd;
		border-radius: 8px;
		margin-bottom: 1.5rem;
	}

	.balance strong {
		font-size: 1.5rem;
	}

	form {
		display: flex;
		gap: 0.5rem;
		margin-bottom: 1.5rem;
		flex-wrap: wrap;
	}

	form input {
		flex: 1;
		min-width: 100px;
	}

	.error {
		color: #b00020;
	}

	.movements {
		list-style: none;
		padding: 0;
		margin: 0;
	}

	.movements li {
		display: flex;
		justify-content: space-between;
		padding: 0.5rem 0;
		border-bottom: 1px solid #eee;
	}

	.movements li.credit span:last-child {
		color: #1a7f37;
	}

	.movements li.debit span:last-child {
		color: #b00020;
	}
</style>
