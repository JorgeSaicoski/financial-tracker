<script>
	import { accountTypeIcons, labelFor, localDate } from '$lib/format.js';

	let { movement, accounts, categories, paymentMethods, onSave, onClose } = $props();

	// Financial fields (amount/currency/timestamp) can't be touched at all
	// for a single installment or one leg of a transfer — the API rejects
	// their presence in the patch outright (see BACK-04/BACK-05).
	const isInstallment = Boolean(movement.credit_card_purchase_id);
	const isTransferLeg = Boolean(movement.transfer_id);
	const financialLocked = isInstallment || isTransferLeg;

	let description = $state(movement.description ?? '');
	let category = $state(movement.category);
	let paymentMethod = $state(movement.payment_method);
	let accountId = $state(movement.account_id ?? '');
	let amountInput = $state((movement.amount / 100).toFixed(2));
	let currencyInput = $state(movement.currency);
	let dateInput = $state(localDate(new Date(movement.timestamp)));
	let submitting = $state(false);
	let error = $state('');

	const selectedAccount = $derived(accounts.find((a) => a.id === accountId));
	const amountCents = $derived(Math.round(parseFloat(amountInput || '0') * 100));
	const effectiveCurrency = $derived(selectedAccount ? selectedAccount.currency : currencyInput);
	const financialChanged = $derived(
		!financialLocked &&
			(amountCents !== movement.amount ||
				effectiveCurrency !== movement.currency ||
				dateInput !== localDate(new Date(movement.timestamp)))
	);
	const willCorrect = $derived(financialChanged && movement.sync_status === 'synced');

	async function handleSubmit(event) {
		event.preventDefault();
		error = '';

		const patch = {
			description: description.trim(),
			category,
			payment_method: paymentMethod,
			account_id: accountId
		};
		if (!financialLocked) {
			if (!amountCents) {
				error = 'Enter a non-zero amount';
				return;
			}
			patch.amount = amountCents;
			patch.currency = effectiveCurrency;
			patch.timestamp = new Date(`${dateInput}T00:00:00`).toISOString();
		}

		submitting = true;
		try {
			await onSave(movement.id, patch);
			onClose();
		} catch (err) {
			error = err.message;
		} finally {
			submitting = false;
		}
	}
</script>

<form class="edit-form" onsubmit={handleSubmit}>
	<div class="form-row">
		<input type="text" placeholder="Description" bind:value={description} />
		<select bind:value={category} aria-label="Category">
			{#each categories as c (c)}
				<option value={c}>{labelFor(c)}</option>
			{/each}
		</select>
	</div>
	<div class="form-row">
		<select bind:value={paymentMethod} aria-label="Payment method">
			{#each paymentMethods as m (m)}
				<option value={m}>{labelFor(m)}</option>
			{/each}
		</select>
		<select bind:value={accountId} aria-label="Account">
			<option value="">No account</option>
			{#each accounts as account (account.id)}
				<option value={account.id}>{accountTypeIcons[account.type] ?? ''} {account.name}</option>
			{/each}
		</select>
	</div>

	{#if financialLocked}
		<p class="hint">
			Only description, category, payment method and account are editable here — cancel the
			{isInstallment ? 'whole purchase' : 'transfer'} instead to change amount, currency or date.
		</p>
	{:else}
		<div class="form-row">
			<input type="number" step="0.01" placeholder="Amount" bind:value={amountInput} required />
			{#if selectedAccount}
				<span class="fixed-currency" title="Currency follows the selected account">
					{selectedAccount.currency.toUpperCase()}
				</span>
			{:else}
				<input type="text" placeholder="Currency" bind:value={currencyInput} />
			{/if}
			<input type="date" bind:value={dateInput} aria-label="Date" />
		</div>
		{#if willCorrect}
			<p class="message notice">
				This movement already synced — saving will reverse it and create a new movement with the
				corrected values.
			</p>
		{/if}
	{/if}

	{#if error}
		<p class="message error">{error}</p>
	{/if}

	<div class="edit-actions">
		<button class="ghost" type="button" onclick={onClose} disabled={submitting}>Discard</button>
		<button class="submit" type="submit" disabled={submitting}>
			{submitting ? 'Saving…' : 'Save'}
		</button>
	</div>
</form>

<style>
	.edit-form {
		flex: 1;
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
		padding: 0.85rem 1rem;
	}

	.hint {
		margin: 0;
		font-size: 0.78rem;
		color: var(--muted);
	}

	.edit-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
	}
</style>
