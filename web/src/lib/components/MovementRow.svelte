<script>
	import { categoryIcons, paymentMethodLabels, formatAmount, labelFor, rowState } from '$lib/format.js';
	import EditMovementForm from './EditMovementForm.svelte';
	import Modal from './Modal.svelte';

	let { movement, accounts, categories, paymentMethods, onCancel, onCancelPurchase, onCancelTransfer, onSave } =
		$props();

	let editing = $state(false);

	const state = $derived(rowState(movement));

	function accountName(id) {
		return accounts.find((a) => a.id === id)?.name;
	}
</script>

<li class={state} class:editing>
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
</li>

{#if editing}
	<Modal title="Edit movement" onClose={() => (editing = false)}>
		<EditMovementForm
			{movement}
			{accounts}
			{categories}
			{paymentMethods}
			{onSave}
			onClose={() => (editing = false)}
		/>
	</Modal>
{/if}

<style>
	li {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.7rem 1rem;
		border-bottom: 1px solid var(--color-border);
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
		background: var(--color-bg);
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

	.chip.installment {
		background: var(--color-bg);
		color: var(--color-text-secondary);
		font-size: 0.72rem;
		vertical-align: middle;
	}

	.chip.sync-pending {
		background: #fef3c7;
		color: #92400e;
	}

	.chip.sync-failed {
		background: var(--color-error-soft);
		color: var(--color-expense);
	}

	.chip.voided-chip,
	.chip.reversed-chip {
		background: var(--color-bg);
		color: var(--color-text-secondary);
	}

	.chip.reversal-chip,
	.chip.transfer-chip {
		background: #dbeafe;
		color: var(--color-info);
	}

	.chip.account-chip {
		background: var(--color-bg);
		color: var(--color-text-secondary);
	}

	.amount {
		font-weight: 700;
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}

	.amount.credit {
		color: var(--color-income);
	}

	.amount.debit {
		color: var(--color-expense);
	}

	.actions {
		display: flex;
		gap: 0.3rem;
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
