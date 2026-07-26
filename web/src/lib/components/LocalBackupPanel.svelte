<script>
	import { onMount } from 'svelte';
	import { getLocalArchiveSetting, setLocalArchiveSetting, exportArchive, importArchive } from '$lib/api.js';
	import {
		encryptArchive,
		decryptArchive,
		saveArchiveFile,
		pickAndReadArchiveFile,
		supportsFileSystemAccess
	} from '$lib/archiveCrypto.js';

	let enabled = $state(false);
	let loadingSetting = $state(true);
	let togglingSetting = $state(false);
	let saving = $state(false);
	let restoring = $state(false);
	let error = $state('');
	let notice = $state('');
	// Session-only: nothing confirms the file still exists or wasn't moved
	// after this, so it's a hint, not a guarantee — see the label below.
	let lastSavedAt = $state(null);

	onMount(async () => {
		try {
			const data = await getLocalArchiveSetting();
			enabled = data.local_archive_enabled;
		} catch {
			// Non-fatal: the save/restore buttons work regardless of this toggle.
		} finally {
			loadingSetting = false;
		}
	});

	async function handleToggle() {
		error = '';
		const previous = enabled;
		enabled = !enabled; // optimistic: flip immediately, roll back on failure
		togglingSetting = true;
		try {
			const data = await setLocalArchiveSetting(enabled);
			enabled = data.local_archive_enabled;
		} catch (err) {
			enabled = previous;
			error = err.message;
		} finally {
			togglingSetting = false;
		}
	}

	async function handleSave() {
		const passphrase = prompt(
			'Choose a passphrase to encrypt this backup.\n\n' +
				'Write it down somewhere safe: if you lose it, this backup is permanently unrecoverable — nobody, including us, can reset it.'
		);
		if (!passphrase) return;

		error = '';
		notice = '';
		saving = true;
		try {
			const archive = await exportArchive();
			const envelope = await encryptArchive(passphrase, archive);
			await saveArchiveFile(envelope);
			lastSavedAt = new Date();
			notice = 'Backup saved and encrypted.';
		} catch (err) {
			if (err?.name !== 'AbortError') error = err.message;
		} finally {
			saving = false;
		}
	}

	async function handleRestore() {
		error = '';
		notice = '';
		restoring = true;
		try {
			const contents = await pickAndReadArchiveFile();
			if (contents === null) return; // user cancelled the picker

			const passphrase = prompt('Enter the passphrase for this backup:');
			if (!passphrase) return;

			const archive = await decryptArchive(passphrase, contents);
			const result = await importArchive(archive);
			const skipped = result.accounts_skipped + result.movements_skipped + result.credit_card_purchases_skipped;
			notice =
				`Restored ${result.accounts_restored} account(s), ${result.movements_restored} movement(s), ` +
				`${result.credit_card_purchases_restored} purchase(s).` +
				(skipped > 0 ? ` (${skipped} already present, skipped.)` : '');
		} catch (err) {
			if (err?.name !== 'AbortError') error = err.message;
		} finally {
			restoring = false;
		}
	}
</script>

<section class="local-backup card">
	<div class="section-head">
		<h2>Local backup</h2>
		<label class="toggle" title="Whether your account is set up for the no-cloud tier — this follows you to any device you log in from">
			<input type="checkbox" checked={enabled} disabled={loadingSetting || togglingSetting} onchange={handleToggle} />
			<span class="track" aria-hidden="true"></span>
			<span class="toggle-label">{enabled ? 'Enabled' : 'Disabled'}</span>
		</label>
	</div>

	<p class="explainer">
		Save an encrypted copy of your accounts, movements and credit-card purchases to a file you
		choose — nobody but you can read it, not even us. There's no server-side copy involved in this
		flow: encryption and decryption both happen in your browser, and the passphrase you set is
		never sent anywhere.
	</p>
	<p class="explainer warning">
		A lost passphrase means the backup is permanently unrecoverable. We have no way to reset it.
	</p>

	{#if error}
		<p class="message error">{error}</p>
	{/if}
	{#if notice}
		<p class="message notice">{notice}</p>
	{/if}

	<div class="actions">
		<button class="ghost" onclick={handleSave} disabled={saving}>
			{saving ? 'Saving…' : 'Save backup now'}
		</button>
		<button class="ghost" onclick={handleRestore} disabled={restoring}>
			{restoring ? 'Restoring…' : 'Restore from backup'}
		</button>
	</div>

	<p class="hint">
		{#if lastSavedAt}
			Last saved locally at {lastSavedAt.toLocaleTimeString()} — a hint only; we can't confirm the
			file still exists or wasn't moved.
		{:else}
			{supportsFileSystemAccess()
				? "You'll be asked where to save each time — nothing is remembered between visits."
				: "Your browser will download the file each time — nothing is remembered between visits."}
		{/if}
	</p>
</section>

<style>
	.explainer {
		font-size: 0.85rem;
		color: var(--color-text-secondary);
		line-height: 1.5;
		margin: var(--space-2) 0 0;
	}

	.explainer.warning {
		color: var(--color-expense);
	}

	.actions {
		display: flex;
		gap: 0.6rem;
		flex-wrap: wrap;
		margin-top: var(--space-2);
	}

	.hint {
		font-size: 0.78rem;
		color: var(--color-text-muted);
		margin: var(--space-1) 0 0;
	}

	.toggle {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		cursor: pointer;
		font-size: 0.8rem;
		color: var(--color-text-secondary);
	}

	.toggle input {
		position: absolute;
		opacity: 0;
		width: 1px;
		height: 1px;
	}

	.track {
		position: relative;
		width: 2.2rem;
		height: 1.3rem;
		border-radius: var(--radius-pill);
		background: var(--color-border);
		transition: background var(--transition-fast);
		flex-shrink: 0;
	}

	.track::after {
		content: '';
		position: absolute;
		top: 2px;
		left: 2px;
		width: calc(1.3rem - 4px);
		height: calc(1.3rem - 4px);
		border-radius: 50%;
		background: var(--color-surface);
		transition: transform var(--transition-fast);
		box-shadow: var(--shadow-soft);
	}

	.toggle input:checked + .track {
		background: var(--color-secondary);
	}

	.toggle input:checked + .track::after {
		transform: translateX(calc(2.2rem - 1.3rem));
	}

	.toggle input:focus-visible + .track {
		outline: none;
		box-shadow: var(--focus-ring);
	}

	.toggle-label {
		font-weight: 500;
	}
</style>
