<script>
	// Authentik redirects here after login (redirect_uri in auth.svelte.js).
	// The actual signinRedirectCallback() call happens in +layout.svelte's
	// onMount → initAuth(), since it needs to run before this page's own
	// state is meaningful either way. Once that settles (authState.ready),
	// just move on to the app.
	import { goto } from '$app/navigation';
	import { authState } from '$lib/auth.svelte.js';

	$effect(() => {
		if (authState.ready) {
			goto('/');
		}
	});
</script>

<div class="callback-screen">
	<p>Completing sign-in…</p>
	{#if authState.error}
		<p class="message error">{authState.error}</p>
	{/if}
</div>

<style>
	.callback-screen {
		min-height: 100vh;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: var(--space-2);
		color: var(--color-text-secondary);
	}
</style>
