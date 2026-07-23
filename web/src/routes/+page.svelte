<script>
	import { onMount } from 'svelte';
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
		getCashflow,
		createTransfer,
		cancelTransfer,
		syncNow
	} from '$lib/api.js';
	import {
		formatAmount,
		labelFor,
		localDate,
		accountTypeIcons,
		categoryIcons,
		paymentMethodLabels
	} from '$lib/format.js';
	import MovementRow from '$lib/components/MovementRow.svelte';
	import TransferForm from '$lib/components/TransferForm.svelte';

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

	let amountInput = $state('');
	let directionInput = $state('expense');
	let currencyInput = $state('usd');
	let descriptionInput = $state('');
	let categoryInput = $state('other');
	let paymentMethodInput = $state('other');
	let installmentsInput = $state(1);
	let accountInput = $state('');
	let submitting = $state(false);
	let syncing = $state(false);

	let showAddAccount = $state(false);
	let accountNameInput = $state('');
	let accountTypeInput = $state('bank');
	let accountCurrencyInput = $state('usd');
	let addingAccount = $state(false);

	const now = new Date();
	let cashflowFrom = $state(localDate(new Date(now.getFullYear(), now.getMonth(), 1)));
	let cashflowTo = $state(localDate(now));
	let cashflow = $state(null);
	let cashflowLoading = $state(false);

	const isCreditCard = $derived(paymentMethodInput === 'credit_card');
	const splittingInstallments = $derived(isCreditCard && Number(installmentsInput) > 1);
	const pendingCount = $derived(
		movements.filter((m) => m.status === 'active' && m.sync_status !== 'synced').length
	);
	const selectedAccount = $derived(accounts.find((a) => a.id === accountInput));

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

	async function handleAddAccount(event) {
		event.preventDefault();
		addingAccount = true;
		error = '';
		try {
			await createAccount({
				name: accountNameInput,
				type: accountTypeInput,
				currency: accountCurrencyInput
			});
			accountNameInput = '';
			showAddAccount = false;
			await loadAccounts();
		} catch (err) {
			error = err.message;
		} finally {
			addingAccount = false;
		}
	}

	async function handleReportBalance(account) {
		const raw = prompt(
			`What does ${account.name} really hold right now, in ${account.currency.toUpperCase()}?`,
			(account.estimated_balance / 100).toFixed(2)
		);
		if (raw === null || raw.trim() === '') return;
		const value = parseFloat(raw.replace(',', '.'));
		if (Number.isNaN(value)) {
			error = 'Enter a number';
			return;
		}
		error = '';
		notice = '';
		try {
			const updated = await reportAccountBalance(account.id, Math.round(value * 100));
			accounts = accounts.map((a) => (a.id === updated.id ? updated : a));
			if (updated.last_return != null) {
				const word = updated.last_return >= 0 ? 'returned' : 'lost';
				notice = `${updated.name} ${word} ${formatAmount(Math.abs(updated.last_return), updated.currency)} since the previous report`;
			} else {
				notice = `Balance recorded for ${updated.name} — report again later to see its return`;
			}
		} catch (err) {
			error = err.message;
		}
	}

	async function handleCashflow() {
		cashflowLoading = true;
		error = '';
		try {
			cashflow = await getCashflow(cashflowFrom, cashflowTo);
		} catch (err) {
			error = err.message;
		} finally {
			cashflowLoading = false;
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
				currency: selectedAccount ? selectedAccount.currency : currencyInput,
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

	onMount(() => {
		load();
		loadCategories();
		loadCurrencies();
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

	<section class="accounts card">
		<div class="section-head">
			<h2>Accounts</h2>
			<button class="ghost" onclick={() => (showAddAccount = !showAddAccount)}>
				{showAddAccount ? 'Close' : '+ Add account'}
			</button>
		</div>

		{#if showAddAccount}
			<form class="add-account" onsubmit={handleAddAccount}>
				<input type="text" placeholder="Name (e.g. Nubank, BTC wallet)" bind:value={accountNameInput} required />
				<select bind:value={accountTypeInput} aria-label="Account type">
					{#each accountTypes.length ? accountTypes : Object.keys(accountTypeIcons) as type (type)}
						<option value={type}>{accountTypeIcons[type] ?? ''} {labelFor(type)}</option>
					{/each}
				</select>
				<select bind:value={accountCurrencyInput} aria-label="Account currency">
					{#each currencies as currency (currency)}
						<option value={currency}>{currency.toUpperCase()}</option>
					{/each}
				</select>
				<button class="submit" type="submit" disabled={addingAccount}>
					{addingAccount ? 'Adding…' : 'Add'}
				</button>
			</form>
		{/if}

		{#if accounts.length === 0}
			<p class="empty small">No accounts yet. Add your bank accounts, wallets and investments to track where the money sits.</p>
		{:else}
			<ul class="account-list">
				{#each accounts as account (account.id)}
					<li>
						<span class="icon" title={labelFor(account.type)}>{accountTypeIcons[account.type] ?? '📦'}</span>
						<div class="details">
							<span class="title">{account.name}</span>
							<span class="meta">
								{#if account.reported_at}
									reported {formatAmount(account.reported_balance, account.currency)}
									on {new Date(account.reported_at).toLocaleDateString()}
									{#if account.movements_since_report !== 0}
										· {account.movements_since_report > 0 ? '+' : ''}{formatAmount(
											account.movements_since_report,
											account.currency
										)} since
									{/if}
								{:else}
									from tracked movements only — report its real balance to start measuring returns
								{/if}
								{#if account.last_return != null}
									<span
										class="chip"
										class:return-pos={account.last_return >= 0}
										class:return-neg={account.last_return < 0}
										title="Balance change the movements don't explain, between your last two reports"
									>
										{account.last_return >= 0 ? '+' : '−'}{formatAmount(
											Math.abs(account.last_return),
											account.currency
										)} return
									</span>
								{/if}
							</span>
						</div>
						<span class="amount">{formatAmount(account.estimated_balance, account.currency)}</span>
						<button class="cancel" title="Report the account's real current balance" onclick={() => handleReportBalance(account)}>
							set balance
						</button>
					</li>
				{/each}
			</ul>
		{/if}
	</section>

	<TransferForm {accounts} onCreate={handleCreateTransfer} />

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
				<select bind:value={currencyInput} aria-label="Currency">
					{#each currencies as currency (currency)}
						<option value={currency}>{currency.toUpperCase()}</option>
					{/each}
				</select>
				<button type="button" class="ghost" title="Add a currency" onclick={handleAddCurrency}>+</button>
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

	<section class="cashflow card">
		<div class="section-head">
			<h2>Cashflow</h2>
			<div class="range">
				<input type="date" bind:value={cashflowFrom} aria-label="From" />
				<span>→</span>
				<input type="date" bind:value={cashflowTo} aria-label="To" />
				<button class="ghost" onclick={handleCashflow} disabled={cashflowLoading}>
					{cashflowLoading ? '…' : 'Calculate'}
				</button>
			</div>
		</div>

		{#if cashflow}
			{#if cashflow.totals.length === 0}
				<p class="empty small">No movements in this period.</p>
			{:else}
				{#each cashflow.totals as flow (flow.currency)}
					<div class="flow-row total">
						<span class="flow-name">{flow.currency.toUpperCase()}</span>
						<span class="flow-in">+{formatAmount(flow.in, flow.currency)}</span>
						<span class="flow-out">−{formatAmount(flow.out, flow.currency)}</span>
						<span class="flow-net" class:credit={flow.net > 0} class:debit={flow.net < 0}>
							{formatAmount(flow.net, flow.currency)}
						</span>
					</div>
				{/each}
				{#if cashflow.by_account.length > 1 || cashflow.by_account.some((f) => f.account_id)}
					<div class="flow-breakdown">
						{#each cashflow.by_account as flow (`${flow.account_id}|${flow.currency}`)}
							<div class="flow-row">
								<span class="flow-name">{flow.name || 'No account'}</span>
								<span class="flow-in">+{formatAmount(flow.in, flow.currency)}</span>
								<span class="flow-out">−{formatAmount(flow.out, flow.currency)}</span>
								<span class="flow-net" class:credit={flow.net > 0} class:debit={flow.net < 0}>
									{formatAmount(flow.net, flow.currency)}
								</span>
							</div>
						{/each}
					</div>
				{/if}
			{/if}
		{:else}
			<p class="empty small">Pick a period and calculate to see money in vs money out.</p>
		{/if}
	</section>

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

	:global(button) {
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

	:global(.form-row) {
		display: flex;
		gap: 0.6rem;
	}

	:global(input),
	:global(select) {
		font: inherit;
		color: var(--text);
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 0.5rem 0.65rem;
		min-width: 0;
	}

	:global(input:focus),
	:global(select:focus) {
		outline: 2px solid var(--accent);
		outline-offset: -1px;
	}

	:global(.form-row input[type='number']),
	:global(.form-row input[type='text']),
	:global(.form-row select) {
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

	:global(.submit) {
		background: var(--accent);
		color: #fff;
		border: none;
		border-radius: 8px;
		padding: 0.6rem;
		font-weight: 600;
		transition: background 0.15s;
	}

	:global(.submit:hover:not(:disabled)) {
		background: var(--accent-hover);
	}

	:global(.submit:disabled) {
		opacity: 0.6;
		cursor: default;
	}

	:global(.message) {
		border-radius: 10px;
		padding: 0.6rem 0.9rem;
		font-size: 0.9rem;
	}

	:global(.message.error) {
		background: var(--red-bg);
		color: var(--red);
	}

	:global(.message.notice) {
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

	.amount {
		font-weight: 600;
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
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

	:global(.card) {
		background: var(--card);
		border: 1px solid var(--border);
		border-radius: 14px;
		padding: 1rem;
		margin-bottom: 1.25rem;
	}

	:global(.section-head) {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 0.6rem;
		flex-wrap: wrap;
	}

	:global(.section-head h2) {
		font-size: 0.95rem;
		margin: 0;
		letter-spacing: -0.01em;
	}

	:global(.ghost) {
		background: none;
		border: 1px solid var(--border);
		color: var(--muted);
		border-radius: 8px;
		padding: 0.35rem 0.7rem;
		font-size: 0.8rem;
		transition:
			color 0.15s,
			border-color 0.15s;
	}

	:global(.ghost:hover:not(:disabled)) {
		color: var(--accent);
		border-color: var(--accent);
	}

	.add-account {
		display: flex;
		gap: 0.6rem;
		flex-wrap: wrap;
		margin-top: 0.8rem;
		background: none;
		border: none;
		padding: 0;
	}

	.add-account input[type='text'] {
		flex: 2;
		min-width: 10rem;
	}

	.add-account select {
		flex: 1;
	}

	.add-account .submit {
		padding: 0.5rem 1rem;
	}

	.account-list {
		list-style: none;
		padding: 0;
		margin: 0.8rem 0 0;
	}

	.account-list li {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.55rem 0;
		border-bottom: 1px solid var(--border);
	}

	.account-list li:last-child {
		border-bottom: none;
		padding-bottom: 0;
	}

	.chip.return-pos {
		background: var(--blue-bg);
		color: var(--green);
	}

	.chip.return-neg {
		background: var(--red-bg);
		color: var(--red);
	}

	:global(.fixed-currency) {
		display: flex;
		align-items: center;
		padding: 0 0.65rem;
		color: var(--muted);
		border: 1px dashed var(--border);
		border-radius: 8px;
		font-size: 0.85rem;
	}

	:global(.empty.small) {
		padding: 0.9rem 0 0.2rem;
		font-size: 0.85rem;
		text-align: left;
	}

	.range {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		color: var(--muted);
		flex-wrap: wrap;
	}

	.range input {
		padding: 0.35rem 0.5rem;
		font-size: 0.85rem;
	}

	.flow-row {
		display: grid;
		grid-template-columns: 1fr auto auto auto;
		gap: 0.9rem;
		align-items: baseline;
		padding: 0.45rem 0;
		border-bottom: 1px solid var(--border);
		font-variant-numeric: tabular-nums;
		font-size: 0.88rem;
	}

	.flow-row:last-child {
		border-bottom: none;
	}

	.flow-row.total {
		font-weight: 600;
	}

	.flow-row.total:first-of-type {
		margin-top: 0.6rem;
	}

	.flow-name {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.flow-in {
		color: var(--green);
	}

	.flow-out {
		color: var(--red);
	}

	.flow-net.credit {
		color: var(--green);
	}

	.flow-net.debit {
		color: var(--red);
	}

	.flow-breakdown {
		margin-top: 0.3rem;
		padding-left: 0.9rem;
		border-left: 2px solid var(--border);
	}

	.flow-breakdown .flow-row {
		font-size: 0.8rem;
		color: var(--muted);
	}
</style>
