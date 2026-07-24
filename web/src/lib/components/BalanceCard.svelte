<script>
	import { formatAmount } from '$lib/format.js';

	let { balance, currency, pendingCount } = $props();
</script>

<section class="balance">
	<div>
		<span class="balance-label">Balance</span>
		{#if pendingCount > 0}
			<span class="pending-note">{pendingCount} awaiting ledger sync</span>
		{/if}
	</div>
	<strong class:negative={balance < 0} class:positive={balance > 0}>
		{formatAmount(balance, currency)}
	</strong>
</section>

<style>
	.balance {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: var(--space-2);
		padding: var(--space-3) var(--space-3);
		background: linear-gradient(135deg, var(--color-primary), var(--color-secondary));
		border-radius: var(--radius-card);
		box-shadow: var(--shadow-soft);
		margin-bottom: var(--space-3);
		color: #fff;
	}

	.balance-label {
		display: block;
		color: rgba(255, 255, 255, 0.8);
		font: var(--text-label);
	}

	.pending-note {
		display: block;
		margin-top: 0.25rem;
		font-size: 0.75rem;
		color: #fde68a;
	}

	.balance strong {
		font: var(--text-amount);
		font-size: 2rem;
		font-variant-numeric: tabular-nums;
		letter-spacing: -0.02em;
		color: #fff;
	}

	.balance strong.negative {
		color: #fecaca;
	}
</style>
