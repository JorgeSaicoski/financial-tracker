<script>
	import { accountTypeIcons, localDate } from '$lib/format.js';

	let { accounts, onCreate } = $props();

	let open = $state(false);
	let fromAccountId = $state('');
	let toAccountId = $state('');
	let amountInput = $state('');
	let description = $state('');
	let dateInput = $state(localDate(new Date()));
	let submitting = $state(false);
	let error = $state('');

	const fromAccount = $derived(accounts.find((a) => a.id === fromAccountId));
	const toAccount = $derived(accounts.find((a) => a.id === toAccountId));
	const sameAccount = $derived(Boolean(fromAccountId) && fromAccountId === toAccountId);
	const currencyMismatch = $derived(
		Boolean(fromAccount) && Boolean(toAccount) && fromAccount.currency !== toAccount.currency
	);
	const canSubmit = $derived(
		fromAccountId && toAccountId && !sameAccount && !currencyMismatch && Number(amountInput) > 0
	);

	async function handleSubmit(event) {
		event.preventDefault();
		if (!canSubmit) return;
		error = '';
		submitting = true;
		try {
			const cents = Math.round(parseFloat(amountInput) * 100);
			const result = await onCreate({
				from_account_id: fromAccountId,
				to_account_id: toAccountId,
				amount: cents,
				description: description.trim(),
				timestamp: new Date(`${dateInput}T00:00:00`).toISOString()
			});
			fromAccountId = '';
			toAccountId = '';
			amountInput = '';
			description = '';
			open = false;
			return result;
		} catch (err) {
			error = err.message;
		} finally {
			submitting = false;
		}
	}
</script>

<section class="card">
	<div class="section-head">
		<h2>Transfer</h2>
		<button class="ghost" onclick={() => (open = !open)}>
			{open ? 'Close' : '+ Transfer'}
		</button>
	</div>

	{#if open}
		<form class="transfer-form" onsubmit={handleSubmit}>
			<div class="form-row">
				<select bind:value={fromAccountId} aria-label="From account" required>
					<option value="" disabled>From account</option>
					{#each accounts as account (account.id)}
						<option value={account.id}>
							{accountTypeIcons[account.type] ?? ''} {account.name} ({account.currency.toUpperCase()})
						</option>
					{/each}
				</select>
				<select bind:value={toAccountId} aria-label="To account" required>
					<option value="" disabled>To account</option>
					{#each accounts as account (account.id)}
						<option value={account.id}>
							{accountTypeIcons[account.type] ?? ''} {account.name} ({account.currency.toUpperCase()})
						</option>
					{/each}
				</select>
			</div>
			<div class="form-row">
				<input type="number" step="0.01" min="0.01" placeholder="Amount" bind:value={amountInput} required />
				<input type="date" bind:value={dateInput} aria-label="Date" />
			</div>
			<div class="form-row">
				<input type="text" placeholder="Description (optional)" bind:value={description} />
			</div>

			{#if sameAccount}
				<p class="hint">Choose two different accounts.</p>
			{:else if currencyMismatch}
				<p class="hint">
					These accounts hold different currencies — cross-currency transfers aren't supported yet.
				</p>
			{/if}
			{#if error}
				<p class="message error">{error}</p>
			{/if}

			<button class="submit" type="submit" disabled={!canSubmit || submitting}>
				{submitting ? 'Transferring…' : 'Transfer'}
			</button>
		</form>
	{:else if accounts.length < 2}
		<p class="empty small">Add at least two accounts to transfer money between them.</p>
	{/if}
</section>

<style>
	.transfer-form {
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
		margin-top: 0.8rem;
	}

	.hint {
		margin: 0;
		font-size: 0.78rem;
		color: var(--muted);
	}
</style>
