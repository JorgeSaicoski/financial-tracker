<script>
	import { getCashflow } from '$lib/api.js';
	import { formatAmount, localDate } from '$lib/format.js';

	const now = new Date();
	let cashflowFrom = $state(localDate(new Date(now.getFullYear(), now.getMonth(), 1)));
	let cashflowTo = $state(localDate(now));
	let cashflow = $state(null);
	let cashflowLoading = $state(false);
	let error = $state('');

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
</script>

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

	{#if error}
		<p class="message error">{error}</p>
	{/if}

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

<style>
	.range {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		color: var(--color-text-secondary);
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
		padding: 0.5rem 0;
		border-bottom: 1px solid var(--color-border);
		font-variant-numeric: tabular-nums;
		font-size: 0.88rem;
	}

	.flow-row:last-child {
		border-bottom: none;
	}

	.flow-row.total {
		font-weight: 700;
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
		color: var(--color-income);
	}

	.flow-out {
		color: var(--color-expense);
	}

	.flow-net.credit {
		color: var(--color-income);
	}

	.flow-net.debit {
		color: var(--color-expense);
	}

	.flow-breakdown {
		margin-top: 0.3rem;
		padding-left: 0.9rem;
		border-left: 2px solid var(--color-border);
	}

	.flow-breakdown .flow-row {
		font-size: 0.8rem;
		color: var(--color-text-secondary);
	}
</style>
