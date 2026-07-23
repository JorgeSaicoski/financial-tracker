<script>
	import { formatAmount, labelFor, accountTypeIcons } from '$lib/format.js';

	let { accounts, accountTypes, currencies, onAddAccount, onReportBalance } = $props();

	let showAddAccount = $state(false);
	let accountNameInput = $state('');
	let accountTypeInput = $state('bank');
	let accountCurrencyInput = $state('usd');
	let addingAccount = $state(false);
	let error = $state('');

	async function handleAddAccount(event) {
		event.preventDefault();
		addingAccount = true;
		error = '';
		try {
			await onAddAccount({
				name: accountNameInput,
				type: accountTypeInput,
				currency: accountCurrencyInput
			});
			accountNameInput = '';
			showAddAccount = false;
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
		try {
			await onReportBalance(account.id, Math.round(value * 100));
		} catch (err) {
			error = err.message;
		}
	}
</script>

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

	{#if error}
		<p class="message error">{error}</p>
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

<style>
	.add-account {
		display: flex;
		gap: 0.6rem;
		flex-wrap: wrap;
		margin-top: var(--space-2);
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
		margin: var(--space-2) 0 0;
	}

	.account-list li {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.6rem 0;
		border-bottom: 1px solid var(--color-border);
	}

	.account-list li:last-child {
		border-bottom: none;
		padding-bottom: 0;
	}

	.account-list .amount {
		margin-left: auto;
	}

	.icon {
		font-size: 1.15rem;
		width: 2rem;
		height: 2rem;
		display: grid;
		place-items: center;
		background: var(--color-bg);
		border-radius: var(--radius-control);
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
		color: var(--color-text-secondary);
		display: flex;
		align-items: center;
		gap: 0.4rem;
		flex-wrap: wrap;
	}

	.chip {
		font: var(--text-label);
		font-size: 0.68rem;
		padding: 0.15rem 0.5rem;
		border-radius: var(--radius-pill);
		white-space: nowrap;
	}

	.chip.return-pos {
		background: var(--color-success-soft);
		color: #166534;
	}

	.chip.return-neg {
		background: var(--color-error-soft);
		color: var(--color-expense);
	}

	.amount {
		font-weight: 700;
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}

	.cancel {
		background: none;
		border: 1px solid var(--color-border);
		color: var(--color-text-secondary);
		border-radius: var(--radius-control);
		padding: 0.3rem 0.6rem;
		font-size: 0.75rem;
		transition:
			color var(--transition-fast),
			border-color var(--transition-fast);
	}

	.cancel:hover {
		color: var(--color-expense);
		border-color: var(--color-expense);
	}
</style>
