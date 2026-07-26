<script>
	import { onDestroy, onMount } from 'svelte';
	import {
		listMovements,
		createMovement,
		updateMovement,
		cancelMovement,
		cancelCreditCardPurchase,
		getCategories,
		getCurrencies,
		addCurrency,
		listAccounts,
		createAccount,
		reportAccountBalance,
		createTransfer,
		cancelTransfer,
		syncNow
	} from '$lib/api.js';
	import { movementCreated, notifyMovementCreated } from '$lib/stores/movementEvents.js';
	import { formatAmount } from '$lib/format.js';
	import AppHeader from '$lib/components/AppHeader.svelte';
	import BalanceCard from '$lib/components/BalanceCard.svelte';
	import AccountsPanel from '$lib/components/AccountsPanel.svelte';
	import LocalBackupPanel from '$lib/components/LocalBackupPanel.svelte';
	import TransferForm from '$lib/components/TransferForm.svelte';
	import AddMovementForm from '$lib/components/AddMovementForm.svelte';
	import CashflowPanel from '$lib/components/CashflowPanel.svelte';
	import MovementRow from '$lib/components/MovementRow.svelte';

	let movements = $state([]);
	let balance = $state(0);
	let loading = $state(true);
	let error = $state('');
	let notice = $state('');

	let categories = $state([]);
	let paymentMethods = $state([]);
	let currencies = $state(['usd', 'brl']);
	let accounts = $state([]);
	let accountTypes = $state([]);
	let currencyInput = $state('usd');

	const pendingCount = $derived(
		movements.filter((m) => m.status === 'active' && m.sync_status !== 'synced').length
	);

	async function load() {
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
		// Movements change account balances too; refresh silently.
		loadAccounts();
	}

	async function loadCategories() {
		try {
			const data = await getCategories();
			categories = data.categories ?? [];
			paymentMethods = data.payment_methods ?? [];
		} catch {
			// The form still works with the defaults baked into the API.
		}
	}

	async function loadCurrencies() {
		try {
			const data = await getCurrencies();
			if (data.currencies?.length) currencies = data.currencies;
		} catch {
			// Keep the usd/brl defaults.
		}
	}

	async function loadAccounts() {
		try {
			const data = await listAccounts();
			accounts = data.accounts ?? [];
			accountTypes = data.account_types ?? [];
		} catch {
			// Accounts are optional; the rest of the page still works.
		}
	}

	async function handleAddCurrency() {
		const code = prompt('New currency code (e.g. btc, eur):');
		if (!code) return;
		error = '';
		try {
			const data = await addCurrency(code.trim().toLowerCase());
			currencies = data.currencies ?? currencies;
			currencyInput = code.trim().toLowerCase();
		} catch (err) {
			error = err.message;
		}
	}

	async function handleAddAccount(input) {
		error = '';
		await createAccount(input);
		await loadAccounts();
	}

	async function handleReportBalance(accountId, cents) {
		error = '';
		notice = '';
		const updated = await reportAccountBalance(accountId, cents);
		accounts = accounts.map((a) => (a.id === updated.id ? updated : a));
		if (updated.last_return != null) {
			const word = updated.last_return >= 0 ? 'returned' : 'lost';
			notice = `${updated.name} ${word} ${formatAmount(Math.abs(updated.last_return), updated.currency)} since the previous report`;
		} else {
			notice = `Balance recorded for ${updated.name} — report again later to see its return`;
		}
	}

	async function handleAddMovement(payload) {
		error = '';
		notice = '';
		await createMovement(payload);
		if (payload.installments > 1) {
			notice = `Purchase split into ${payload.installments} monthly installments`;
		}
		// The reload happens via the movementCreated subscription below, the
		// same path QuickAdd's submits use (FRONT-09) — one refresh
		// mechanism regardless of which form created the movement.
		notifyMovementCreated();
	}

	async function handleCancel(movement) {
		if (!confirm('Cancel this movement? If it already reached the ledger, a reversal is created.'))
			return;
		error = '';
		notice = '';
		try {
			const result = await cancelMovement(movement.id);
			notice = result.reversal
				? 'Movement reversed (it had already synced to the ledger)'
				: 'Movement voided';
			await load();
		} catch (err) {
			error = err.message;
		}
	}

	async function handleCancelPurchase(purchaseId) {
		if (!confirm('Cancel ALL installments of this purchase?')) return;
		error = '';
		notice = '';
		try {
			const result = await cancelCreditCardPurchase(purchaseId);
			notice = `Purchase cancelled: ${result.voided.length} voided, ${result.reversals.length} reversed`;
			await load();
		} catch (err) {
			error = err.message;
		}
	}

	async function handleSync() {
		error = '';
		notice = '';
		try {
			const summary = await syncNow();
			notice =
				summary.synced === 0 && summary.failed === 0
					? 'Nothing due to sync'
					: `Sync: ${summary.synced} pushed, ${summary.failed} failed`;
			await load();
		} catch (err) {
			error = err.message;
		}
	}

	async function handleUpdateMovement(id, patch) {
		error = '';
		notice = '';
		const result = await updateMovement(id, patch);
		notice = result.replacement
			? 'Correction recorded: the original was reversed and a replacement created with the new values'
			: 'Movement updated';
		await load();
		return result;
	}

	async function handleCreateTransfer(input) {
		error = '';
		notice = '';
		const result = await createTransfer(input);
		notice = 'Transfer complete';
		await load();
		return result;
	}

	async function handleCancelTransfer(transferId) {
		if (!confirm('Cancel this transfer? Both legs will be voided or reversed as needed.')) return;
		error = '';
		notice = '';
		try {
			await cancelTransfer(transferId);
			notice = 'Transfer cancelled (both legs)';
			await load();
		} catch (err) {
			error = err.message;
		}
	}

	// Skip the store's initial callback (fires immediately on subscribe with
	// the current count) so mount doesn't double-load; react only to
	// genuine new movements — created here, or via QuickAdd from any route.
	let skippedInitialMovementEvent = false;
	const unsubscribeMovementCreated = movementCreated.subscribe(() => {
		if (!skippedInitialMovementEvent) {
			skippedInitialMovementEvent = true;
			return;
		}
		load();
	});
	onDestroy(unsubscribeMovementCreated);

	onMount(() => {
		load();
		loadCategories();
		loadCurrencies();
	});
</script>

<main>
	<AppHeader onSync={handleSync} />

	<BalanceCard {balance} currency={currencyInput} {pendingCount} />

	<CashflowPanel />

	<AccountsPanel {accounts} {accountTypes} {currencies} onAddAccount={handleAddAccount} onReportBalance={handleReportBalance} />

	<LocalBackupPanel />

	<TransferForm {accounts} onCreate={handleCreateTransfer} />

	<AddMovementForm
		{categories}
		{paymentMethods}
		{currencies}
		{accounts}
		bind:currency={currencyInput}
		onSubmit={handleAddMovement}
		onAddCurrency={handleAddCurrency}
	/>

	{#if error}
		<p class="message error">{error}</p>
	{/if}
	{#if notice}
		<p class="message notice">{notice}</p>
	{/if}

	{#if loading}
		<p class="empty">Loading…</p>
	{:else if movements.length === 0}
		<p class="empty">No movements yet.</p>
	{:else}
		<ul class="movements">
			{#each movements as movement (movement.id)}
				<MovementRow
					{movement}
					{accounts}
					{categories}
					{paymentMethods}
					onCancel={handleCancel}
					onCancelPurchase={handleCancelPurchase}
					onCancelTransfer={handleCancelTransfer}
					onSave={handleUpdateMovement}
				/>
			{/each}
		</ul>
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

	.empty {
		text-align: center;
		color: var(--color-text-secondary);
		padding: var(--space-3) 0;
	}

	.movements {
		list-style: none;
		padding: 0;
		margin: 0;
		background: var(--color-surface);
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		box-shadow: var(--shadow-soft);
		overflow: hidden;
	}
</style>
