<script>
	import { onMount } from 'svelte';
	import {
		listMovements,
		createMovement,
		cancelMovement,
		cancelCreditCardPurchase,
		getCategories,
		syncNow
	} from '$lib/api.js';

	let movements = $state([]);
	let balance = $state(0);
	let loading = $state(true);
	let error = $state('');
	let notice = $state('');

	let categories = $state([]);
	let paymentMethods = $state([]);

	let amountInput = $state('');
	let directionInput = $state('expense');
	let currencyInput = $state('usd');
	let descriptionInput = $state('');
	let categoryInput = $state('other');
	let paymentMethodInput = $state('other');
	let installmentsInput = $state(1);
	let submitting = $state(false);
	let syncing = $state(false);

	const categoryIcons = {
		food: '🍽️',
		transport: '🚌',
		housing: '🏠',
		utilities: '💡',
		health: '🏥',
		entertainment: '🎬',
		shopping: '🛍️',
		education: '📚',
		income: '💰',
		transfer: '🔁',
		other: '📦'
	};

	const paymentMethodLabels = {
		cash: 'Cash',
		debit_card: 'Debit card',
		credit_card: 'Credit card',
		pix: 'Pix',
		bank_transfer: 'Bank transfer',
		other: 'Other'
	};

	const isCreditCard = $derived(paymentMethodInput === 'credit_card');
	const pendingCount = $derived(
		movements.filter((m) => m.status === 'active' && m.sync_status !== 'synced').length
	);

	function formatAmount(cents, currency) {
		return new Intl.NumberFormat('en-US', {
			style: 'currency',
			currency: currency.toUpperCase()
		}).format(cents / 100);
	}

	function labelFor(category) {
		return category.charAt(0).toUpperCase() + category.slice(1);
	}

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
		notice = '';
		try {
			await createMovement({
				amount: signedAmount,
				currency: currencyInput,
				description: descriptionInput.trim(),
				category: categoryInput,
				payment_method: paymentMethodInput,
				installments
			});
			amountInput = '';
			descriptionInput = '';
			installmentsInput = 1;
			if (installments > 1) {
				notice = `Purchase split into ${installments} monthly installments`;
			}
			await load();
		} catch (err) {
			error = err.message;
		} finally {
			submitting = false;
		}
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

	async function handleCancelPurchase(movement) {
		if (!confirm('Cancel ALL installments of this purchase?')) return;
		error = '';
		notice = '';
		try {
			const result = await cancelCreditCardPurchase(movement.credit_card_purchase_id);
			notice = `Purchase cancelled: ${result.voided.length} voided, ${result.reversals.length} reversed`;
			await load();
		} catch (err) {
			error = err.message;
		}
	}

	async function handleSync() {
		syncing = true;
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
		} finally {
			syncing = false;
		}
	}

	function rowState(movement) {
		if (movement.status === 'voided') return 'voided';
		if (movement.cancels_movement_id) return 'reversal';
		if (movement.reversed_by_movement_id) return 'reversed';
		return 'active';
	}

	onMount(() => {
		load();
		loadCategories();
	});
</script>

<main>
	<header>
		<h1>Financial Tracker</h1>
		<button class="sync" onclick={handleSync} disabled={syncing}>
			{#if syncing}Syncing…{:else}⟳ Sync now{/if}
		</button>
	</header>

	<section class="balance">
		<div>
			<span class="balance-label">Balance</span>
			{#if pendingCount > 0}
				<span class="pending-note">{pendingCount} awaiting ledger sync</span>
			{/if}
		</div>
		<strong class:negative={balance < 0} class:positive={balance > 0}>
			{formatAmount(balance, currencyInput)}
		</strong>
	</section>

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
			<select bind:value={currencyInput} aria-label="Currency">
				<option value="usd">USD</option>
				<option value="brl">BRL</option>
			</select>
		</div>
		<div class="form-row">
			<input type="text" placeholder="Description (optional)" bind:value={descriptionInput} />
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
				{@const state = rowState(movement)}
				<li class={state}>
					<span class="icon" title={labelFor(movement.category)}>
						{categoryIcons[movement.category] ?? '📦'}
					</span>
					<div class="details">
						<span class="title">
							{movement.description || labelFor(movement.category)}
							{#if movement.installment_number}
								<span class="chip installment">#{movement.installment_number}</span>
							{/if}
						</span>
						<span class="meta">
							{new Date(movement.timestamp).toLocaleDateString(undefined, {
								day: 'numeric',
								month: 'short',
								year: 'numeric'
							})}
							· {paymentMethodLabels[movement.payment_method] ?? movement.payment_method}
							{#if state === 'voided'}
								<span class="chip voided-chip">voided</span>
							{:else if state === 'reversal'}
								<span class="chip reversal-chip">reversal</span>
							{:else if state === 'reversed'}
								<span class="chip reversed-chip">reversed</span>
							{/if}
							{#if movement.status === 'active'}
								{#if movement.sync_status === 'pending'}
									<span class="chip sync-pending" title="Not yet in ledger-service">pending sync</span>
								{:else if movement.sync_status === 'failed'}
									<span class="chip sync-failed" title="Last sync attempt failed; will retry">sync failed</span>
								{/if}
							{/if}
						</span>
					</div>
					<span class="amount" class:credit={movement.amount > 0} class:debit={movement.amount < 0}>
						{formatAmount(movement.amount, movement.currency)}
					</span>
					{#if state === 'active'}
						<div class="actions">
							<button
								class="cancel"
								title="Cancel this movement"
								onclick={() => handleCancel(movement)}>✕</button
							>
							{#if movement.credit_card_purchase_id}
								<button
									class="cancel all"
									title="Cancel the whole purchase (all installments)"
									onclick={() => handleCancelPurchase(movement)}>✕ all</button
								>
							{/if}
						</div>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
</main>

<style>
	:global(body) {
		margin: 0;
		background: var(--bg);
	}

	main {
		--bg: #f4f5f7;
		--card: #ffffff;
		--border: #e3e5e8;
		--text: #1c1e21;
		--muted: #6b7280;
		--accent: #2563eb;
		--accent-hover: #1d4ed8;
		--green: #15803d;
		--red: #b91c1c;
		--amber-bg: #fef3c7;
		--amber-text: #92400e;
		--red-bg: #fee2e2;
		--blue-bg: #dbeafe;
		--blue-text: #1e40af;
		--gray-bg: #e5e7eb;

		max-width: 560px;
		margin: 0 auto;
		padding: 2rem 1rem 4rem;
		font-family: system-ui, -apple-system, sans-serif;
		color: var(--text);
	}

	@media (prefers-color-scheme: dark) {
		main {
			--bg: #101214;
			--card: #1a1d21;
			--border: #2c3138;
			--text: #e6e8ea;
			--muted: #9aa1a9;
			--accent: #3b82f6;
			--accent-hover: #60a5fa;
			--green: #4ade80;
			--red: #f87171;
			--amber-bg: #453308;
			--amber-text: #fcd34d;
			--red-bg: #4c1414;
			--blue-bg: #172a54;
			--blue-text: #93c5fd;
			--gray-bg: #32373d;
		}
	}

	header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 1.25rem;
	}

	h1 {
		font-size: 1.35rem;
		margin: 0;
		letter-spacing: -0.02em;
	}

	button {
		font: inherit;
		cursor: pointer;
	}

	.sync {
		background: var(--card);
		border: 1px solid var(--border);
		color: var(--text);
		border-radius: 8px;
		padding: 0.45rem 0.9rem;
		transition: border-color 0.15s;
	}

	.sync:hover:not(:disabled) {
		border-color: var(--accent);
	}

	.sync:disabled {
		opacity: 0.6;
		cursor: default;
	}

	.balance {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 1.1rem 1.25rem;
		background: var(--card);
		border: 1px solid var(--border);
		border-radius: 14px;
		margin-bottom: 1.25rem;
	}

	.balance-label {
		display: block;
		color: var(--muted);
		font-size: 0.85rem;
	}

	.pending-note {
		display: block;
		margin-top: 0.2rem;
		font-size: 0.75rem;
		color: var(--amber-text);
	}

	.balance strong {
		font-size: 1.75rem;
		letter-spacing: -0.03em;
	}

	.balance strong.negative {
		color: var(--red);
	}

	.balance strong.positive {
		color: var(--green);
	}

	form {
		background: var(--card);
		border: 1px solid var(--border);
		border-radius: 14px;
		padding: 1rem;
		margin-bottom: 1.25rem;
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
	}

	.form-row {
		display: flex;
		gap: 0.6rem;
	}

	input,
	select {
		font: inherit;
		color: var(--text);
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 0.5rem 0.65rem;
		min-width: 0;
	}

	input:focus,
	select:focus {
		outline: 2px solid var(--accent);
		outline-offset: -1px;
	}

	.form-row input[type='number'],
	.form-row input[type='text'],
	.form-row select {
		flex: 1;
	}

	.installments {
		display: flex;
		align-items: center;
		gap: 0.3rem;
		color: var(--muted);
	}

	.installments input {
		width: 4rem;
	}

	.submit {
		background: var(--accent);
		color: #fff;
		border: none;
		border-radius: 8px;
		padding: 0.6rem;
		font-weight: 600;
		transition: background 0.15s;
	}

	.submit:hover:not(:disabled) {
		background: var(--accent-hover);
	}

	.submit:disabled {
		opacity: 0.6;
		cursor: default;
	}

	.message {
		border-radius: 10px;
		padding: 0.6rem 0.9rem;
		font-size: 0.9rem;
	}

	.message.error {
		background: var(--red-bg);
		color: var(--red);
	}

	.message.notice {
		background: var(--blue-bg);
		color: var(--blue-text);
	}

	.empty {
		text-align: center;
		color: var(--muted);
		padding: 2rem 0;
	}

	.movements {
		list-style: none;
		padding: 0;
		margin: 0;
		background: var(--card);
		border: 1px solid var(--border);
		border-radius: 14px;
		overflow: hidden;
	}

	.movements li {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.7rem 1rem;
		border-bottom: 1px solid var(--border);
	}

	.movements li:last-child {
		border-bottom: none;
	}

	.movements li.voided {
		opacity: 0.55;
	}

	.movements li.voided .title,
	.movements li.voided .amount {
		text-decoration: line-through;
	}

	.icon {
		font-size: 1.15rem;
		width: 2rem;
		height: 2rem;
		display: grid;
		place-items: center;
		background: var(--bg);
		border-radius: 10px;
		flex-shrink: 0;
	}

	.details {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
	}

	.title {
		font-weight: 500;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.meta {
		font-size: 0.78rem;
		color: var(--muted);
		display: flex;
		align-items: center;
		gap: 0.4rem;
		flex-wrap: wrap;
	}

	.chip {
		font-size: 0.68rem;
		font-weight: 600;
		padding: 0.1rem 0.45rem;
		border-radius: 99px;
		white-space: nowrap;
	}

	.chip.installment {
		background: var(--gray-bg);
		color: var(--muted);
		font-size: 0.72rem;
		vertical-align: middle;
	}

	.chip.sync-pending {
		background: var(--amber-bg);
		color: var(--amber-text);
	}

	.chip.sync-failed {
		background: var(--red-bg);
		color: var(--red);
	}

	.chip.voided-chip,
	.chip.reversed-chip {
		background: var(--gray-bg);
		color: var(--muted);
	}

	.chip.reversal-chip {
		background: var(--blue-bg);
		color: var(--blue-text);
	}

	.amount {
		font-weight: 600;
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}

	.amount.credit {
		color: var(--green);
	}

	.amount.debit {
		color: var(--red);
	}

	.actions {
		display: flex;
		gap: 0.3rem;
	}

	.cancel {
		background: none;
		border: 1px solid var(--border);
		color: var(--muted);
		border-radius: 7px;
		padding: 0.25rem 0.5rem;
		font-size: 0.75rem;
		transition:
			color 0.15s,
			border-color 0.15s;
	}

	.cancel:hover {
		color: var(--red);
		border-color: var(--red);
	}
</style>
