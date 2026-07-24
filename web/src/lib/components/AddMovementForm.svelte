<script>
	import { labelFor, accountTypeIcons, categoryIcons, paymentMethodLabels } from '$lib/format.js';

	let {
		categories,
		paymentMethods,
		currencies,
		accounts,
		currency = $bindable(),
		onSubmit,
		onAddCurrency
	} = $props();

	let amountInput = $state('');
	let directionInput = $state('expense');
	let descriptionInput = $state('');
	let categoryInput = $state('other');
	let paymentMethodInput = $state('other');
	let installmentsInput = $state(1);
	let accountInput = $state('');
	let submitting = $state(false);
	let error = $state('');

	const isCreditCard = $derived(paymentMethodInput === 'credit_card');
	const splittingInstallments = $derived(isCreditCard && Number(installmentsInput) > 1);
	const selectedAccount = $derived(accounts.find((a) => a.id === accountInput));

	async function handleSubmit(event) {
		event.preventDefault();

		const cents = Math.round(parseFloat(amountInput) * 100);
		if (!cents) {
			error = 'Enter a non-zero amount';
			return;
		}
		const signedAmount = directionInput === 'expense' ? -Math.abs(cents) : Math.abs(cents);
		const installments = isCreditCard ? Math.max(1, Number(installmentsInput) || 1) : 1;

		submitting = true;
		error = '';
		try {
			await onSubmit({
				amount: signedAmount,
				currency: selectedAccount ? selectedAccount.currency : currency,
				description: descriptionInput.trim(),
				category: categoryInput,
				payment_method: paymentMethodInput,
				installments,
				// Installment purchases are future bills, not money leaving
				// an account today — the API rejects the combination.
				account_id: installments > 1 ? undefined : accountInput || undefined
			});
			amountInput = '';
			descriptionInput = '';
			installmentsInput = 1;
		} catch (err) {
			error = err.message;
		} finally {
			submitting = false;
		}
	}
</script>

<form onsubmit={handleSubmit}>
	<div class="form-row">
		<input
			type="number"
			step="0.01"
			min="0"
			placeholder="Amount"
			bind:value={amountInput}
			required
		/>
		<select bind:value={directionInput} aria-label="Direction">
			<option value="expense">Expense</option>
			<option value="income">Income</option>
		</select>
		{#if selectedAccount}
			<span class="fixed-currency" title="Currency follows the selected account">
				{selectedAccount.currency.toUpperCase()}
			</span>
		{:else}
			<select bind:value={currency} aria-label="Currency">
				{#each currencies as c (c)}
					<option value={c}>{c.toUpperCase()}</option>
				{/each}
			</select>
			<button type="button" class="ghost" title="Add a currency" onclick={onAddCurrency}>+</button>
		{/if}
	</div>
	<div class="form-row">
		<input type="text" placeholder="Description (optional)" bind:value={descriptionInput} />
		<select bind:value={accountInput} aria-label="Account" disabled={splittingInstallments}>
			<option value="">No account</option>
			{#each accounts as account (account.id)}
				<option value={account.id}>{accountTypeIcons[account.type] ?? ''} {account.name}</option>
			{/each}
		</select>
	</div>
	<div class="form-row">
		<select bind:value={categoryInput} aria-label="Category">
			{#each categories as category (category)}
				<option value={category}>{categoryIcons[category] ?? ''} {labelFor(category)}</option>
			{/each}
		</select>
		<select bind:value={paymentMethodInput} aria-label="Payment method">
			{#each paymentMethods as method (method)}
				<option value={method}>{paymentMethodLabels[method] ?? method}</option>
			{/each}
		</select>
		{#if isCreditCard}
			<label class="installments">
				<input type="number" min="1" max="48" bind:value={installmentsInput} />
				<span>×</span>
			</label>
		{/if}
	</div>

	{#if error}
		<p class="message error">{error}</p>
	{/if}

	<button class="submit" type="submit" disabled={submitting}>
		{#if submitting}
			Adding…
		{:else if isCreditCard && installmentsInput > 1}
			Add in {installmentsInput} installments
		{:else}
			Add movement
		{/if}
	</button>
</form>

<style>
	form {
		background: var(--color-surface);
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		padding: var(--space-2);
		margin-bottom: var(--space-3);
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
	}

	.installments {
		display: flex;
		align-items: center;
		gap: 0.3rem;
		color: var(--color-text-secondary);
	}

	.installments input {
		width: 4rem;
	}
</style>
