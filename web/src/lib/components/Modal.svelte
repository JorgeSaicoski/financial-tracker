<script>
	import { onMount, onDestroy, tick } from 'svelte';

	let { title, onClose, children } = $props();

	let modalEl;
	let closeEl;
	let lastActive;

	onMount(async () => {
		lastActive = document.activeElement;
		await tick();
		closeEl?.focus();
	});

	onDestroy(() => {
		if (lastActive instanceof HTMLElement) lastActive.focus();
	});

	function handleKeydown(event) {
		if (event.key === 'Escape') {
			event.preventDefault();
			onClose();
			return;
		}

		if (event.key !== 'Tab' || !modalEl) return;

		const focusables = modalEl.querySelectorAll(
			'a[href], button:not([disabled]), textarea, input, select, [tabindex]:not([tabindex="-1"])'
		);
		if (focusables.length === 0) return;

		const first = focusables[0];
		const last = focusables[focusables.length - 1];

		if (event.shiftKey && document.activeElement === first) {
			event.preventDefault();
			last.focus();
		} else if (!event.shiftKey && document.activeElement === last) {
			event.preventDefault();
			first.focus();
		}
	}
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- Escape (below) and the close button already give keyboard users a way
     out; the backdrop click is a mouse-only convenience on top of that. -->
<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="overlay" onclick={onClose} role="presentation">
	<div
		class="modal"
		bind:this={modalEl}
		onclick={(event) => event.stopPropagation()}
		role="dialog"
		aria-modal="true"
		aria-label={title}
	>
		<div class="modal-head">
			<h2>{title}</h2>
			<button bind:this={closeEl} class="close" type="button" onclick={onClose} aria-label="Close">✕</button>
		</div>
		{@render children()}
	</div>
</div>

<style>
	.overlay {
		position: fixed;
		inset: 0;
		background: rgba(16, 42, 67, 0.45);
		display: flex;
		align-items: center;
		justify-content: center;
		padding: var(--space-2);
		z-index: 100;
	}

	.modal {
		width: 100%;
		max-width: 440px;
		max-height: calc(100vh - var(--space-4));
		overflow-y: auto;
		background: var(--color-surface);
		border-radius: var(--radius-card);
		box-shadow: 0 24px 48px rgba(16, 42, 67, 0.28);
	}

	.modal-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-2);
		padding: var(--space-2) var(--space-2) 0;
	}

	.modal-head h2 {
		font: var(--text-section-title);
		font-size: 1.05rem;
		margin: 0;
		color: var(--color-text-primary);
	}

	.close {
		background: none;
		border: none;
		color: var(--color-text-secondary);
		font-size: 1rem;
		line-height: 1;
		padding: 0.35rem;
		border-radius: var(--radius-control);
		transition: color var(--transition-fast), background var(--transition-fast);
	}

	.close:hover {
		color: var(--color-text-primary);
		background: var(--color-bg);
	}

	.close:focus-visible {
		outline: none;
		box-shadow: var(--focus-ring);
	}
</style>
