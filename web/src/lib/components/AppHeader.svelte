<script>
	import { authState, logout } from '$lib/auth.svelte.js';

	let { onSync } = $props();

	let syncing = $state(false);

	async function handleClick() {
		syncing = true;
		try {
			await onSync();
		} finally {
			syncing = false;
		}
	}

	// From the ID token's claims (not a separate userinfo call) — see
	// auth.svelte.js.
	const displayName = $derived(
		authState.user?.profile?.name ?? authState.user?.profile?.email ?? ''
	);
</script>

<header>
	<h1>Financial Tracker</h1>
	<div class="header-actions">
		{#if authState.authEnabled && displayName}
			<span class="user-chip">{displayName}</span>
		{/if}
		<button class="sync" onclick={handleClick} disabled={syncing}>
			{#if syncing}Syncing…{:else}⟳ Sync now{/if}
		</button>
		{#if authState.authEnabled}
			<button class="ghost" onclick={logout}>Log out</button>
		{/if}
	</div>
</header>

<style>
	header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: var(--space-3);
	}

	h1 {
		font: var(--text-page-title);
		margin: 0;
		color: var(--color-primary);
		letter-spacing: -0.02em;
	}

	.header-actions {
		display: flex;
		align-items: center;
		gap: 0.6rem;
	}

	.user-chip {
		font: var(--text-label);
		color: var(--color-text-secondary);
	}

	.sync {
		background: var(--color-surface);
		border: 1px solid var(--color-border);
		color: var(--color-primary);
		border-radius: var(--radius-control);
		padding: 0.5rem 1rem;
		font-weight: 600;
		transition:
			border-color var(--transition-fast),
			color var(--transition-fast);
	}

	.sync:hover:not(:disabled) {
		border-color: var(--color-secondary);
		color: var(--color-secondary);
	}

	.sync:focus-visible {
		outline: none;
		box-shadow: var(--focus-ring);
	}

	.sync:disabled {
		opacity: 0.6;
		cursor: default;
	}
</style>
