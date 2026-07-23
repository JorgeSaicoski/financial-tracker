<script>
	import { categoryIcons, paymentMethodLabels, formatAmount, labelFor, rowState } from '$lib/format.js';
	import EditMovementForm from './EditMovementForm.svelte';

	let { movement, accounts, categories, paymentMethods, onCancel, onCancelPurchase, onCancelTransfer, onSave } =
		$props();

	let editing = $state(false);

	const state = $derived(rowState(movement));

	function accountName(id) {
		return accounts.find((a) => a.id === id)?.name;
	}
</script>

<li class={state} class:editing>
	{#if editing}
		<EditMovementForm
			{movement}
			{accounts}
			{categories}
			{paymentMethods}
			{onSave}
			onClose={() => (editing = false)}
		/>
	{:else}
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
				{#if movement.account_id && accountName(movement.account_id)}
					<span class="chip account-chip">{accountName(movement.account_id)}</span>
				{/if}
				{#if movement.transfer_id}
					<span class="chip transfer-chip" title="Part of a transfer">⇄ transfer</span>
				{/if}
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
				<button class="cancel" title="Edit this movement" onclick={() => (editing = true)}>✎</button>
				{#if movement.transfer_id}
					<button
						class="cancel all"
						title="Cancel the whole transfer (both legs)"
						onclick={() => onCancelTransfer(movement.transfer_id)}>⇄ cancel</button
					>
				{:else}
					<button class="cancel" title="Cancel this movement" onclick={() => onCancel(movement)}>✕</button>
					{#if movement.credit_card_purchase_id}
						<button
							class="cancel all"
							title="Cancel the whole purchase (all installments)"
							onclick={() => onCancelPurchase(movement.credit_card_purchase_id)}>✕ all</button
						>
					{/if}
				{/if}
			</div>
		{/if}
	{/if}
</li>

<style>
	li {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.7rem 1rem;
		border-bottom: 1px solid var(--border);
	}

	li:last-child {
		border-bottom: none;
	}

	li.voided {
		opacity: 0.55;
	}

	li.voided .title,
	li.voided .amount {
		text-decoration: line-through;
	}

	li.editing {
		align-items: stretch;
		padding: 0;
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

	.chip.reversal-chip,
	.chip.transfer-chip {
		background: var(--blue-bg);
		color: var(--blue-text);
	}

	.chip.account-chip {
		background: var(--gray-bg);
		color: var(--muted);
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
