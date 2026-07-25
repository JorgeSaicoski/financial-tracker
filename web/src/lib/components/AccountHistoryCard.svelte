<script>
	import { formatAmount, labelFor, accountTypeIcons, localDate } from '$lib/format.js';
	import { getAccountSnapshots } from '$lib/api.js';

	let { account, onReportBalance } = $props();

	let amountInput = $state('');
	let dateInput = $state(localDate(new Date()));
	let reporting = $state(false);
	let reportError = $state('');

	let showHistory = $state(false);
	let snapshots = $state([]);
	let loadingHistory = $state(false);
	let historyError = $state('');
	let historyLoaded = false;

	async function loadHistory() {
		loadingHistory = true;
		historyError = '';
		try {
			const data = await getAccountSnapshots(account.id);
			snapshots = data.snapshots ?? [];
			historyLoaded = true;
		} catch (err) {
			historyError = err.message;
		} finally {
			loadingHistory = false;
		}
	}

	function toggleHistory() {
		showHistory = !showHistory;
		if (showHistory && !historyLoaded) {
			loadHistory();
		}
	}

	async function handleReport(event) {
		event.preventDefault();
		const value = parseFloat(amountInput.replace(',', '.'));
		if (Number.isNaN(value)) {
			reportError = 'Enter a number';
			return;
		}
		reportError = '';
		reporting = true;
		try {
			const timestamp = new Date(`${dateInput}T00:00:00Z`).toISOString();
			await onReportBalance(account.id, Math.round(value * 100), timestamp);
			amountInput = '';
			// The report just posted belongs in the history too — reload it
			// if it's open, rather than showing stale entries.
			if (showHistory) {
				historyLoaded = false;
				await loadHistory();
			}
		} catch (err) {
			reportError = err.message;
		} finally {
			reporting = false;
		}
	}
</script>

<section class="account-history card">
	<div class="section-head">
		<div class="title-group">
			<span class="icon" title={labelFor(account.type)}>{accountTypeIcons[account.type] ?? '📦'}</span>
			<h2>{account.name}</h2>
		</div>
		<span class="amount">{formatAmount(account.estimated_balance, account.currency)}</span>
	</div>

	<p class="meta">
		{#if account.reported_at}
			reported {formatAmount(account.reported_balance, account.currency)} on {new Date(
				account.reported_at
			).toLocaleDateString()}
			{#if account.last_return != null}
				<span
					class="chip"
					class:return-pos={account.last_return >= 0}
					class:return-neg={account.last_return < 0}
				>
					{account.last_return >= 0 ? '+' : '−'}{formatAmount(Math.abs(account.last_return), account.currency)} return
				</span>
			{/if}
		{:else}
			from tracked movements only — report its real balance below to start measuring returns
		{/if}
	</p>

	<form class="report-form" onsubmit={handleReport}>
		<input
			type="number"
			step="0.01"
			placeholder={`Balance in ${account.currency.toUpperCase()}`}
			bind:value={amountInput}
			required
		/>
		<input type="date" bind:value={dateInput} aria-label="Report date" />
		<button class="submit" type="submit" disabled={reporting}>
			{reporting ? 'Reporting…' : 'Report balance'}
		</button>
	</form>
	{#if reportError}
		<p class="message error">{reportError}</p>
	{/if}

	<button class="ghost history-toggle" onclick={toggleHistory}>
		{showHistory ? 'Hide history' : 'Show history'}
	</button>

	{#if showHistory}
		{#if loadingHistory}
			<p class="empty small">Loading…</p>
		{:else if historyError}
			<p class="message error">{historyError}</p>
		{:else if snapshots.length === 0}
			<p class="empty small">No balance reports yet.</p>
		{:else}
			<ul class="history-list">
				{#each snapshots as snap (snap.id)}
					<li>
						<span class="date">{new Date(snap.timestamp).toLocaleDateString()}</span>
						<span class="balance">{formatAmount(snap.balance, account.currency)}</span>
						<span class="chip" class:return-pos={snap.return >= 0} class:return-neg={snap.return < 0}>
							{snap.return >= 0 ? '+' : '−'}{formatAmount(Math.abs(snap.return), account.currency)}
						</span>
					</li>
				{/each}
			</ul>
		{/if}
	{/if}
</section>

<style>
	.title-group {
		display: flex;
		align-items: center;
		gap: 0.6rem;
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

	.amount {
		font-weight: 700;
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}

	.meta {
		font-size: 0.85rem;
		color: var(--color-text-secondary);
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
		margin: var(--space-1) 0 0;
	}

	.chip {
		font: var(--text-label);
		font-size: 0.7rem;
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

	.report-form {
		display: flex;
		gap: 0.6rem;
		flex-wrap: wrap;
		margin-top: var(--space-2);
	}

	.report-form input[type='number'] {
		flex: 2;
		min-width: 9rem;
	}

	.report-form input[type='date'] {
		flex: 1;
		min-width: 8rem;
	}

	.report-form .submit {
		padding: 0.5rem 1rem;
	}

	.history-toggle {
		margin-top: var(--space-2);
	}

	.history-list {
		list-style: none;
		padding: 0;
		margin: var(--space-2) 0 0;
	}

	.history-list li {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.5rem 0;
		border-bottom: 1px solid var(--color-border);
		font-size: 0.85rem;
	}

	.history-list li:last-child {
		border-bottom: none;
		padding-bottom: 0;
	}

	.history-list .date {
		color: var(--color-text-secondary);
		flex: 1;
	}

	.history-list .balance {
		font-weight: 600;
		font-variant-numeric: tabular-nums;
	}
</style>
