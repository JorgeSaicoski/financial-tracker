// Client-side encryption for BACK-15's "no cloud" local archive. Nothing
// here ever leaves the page unencrypted, and the passphrase itself never
// touches the network — only the backend's plaintext GET /export/archive
// response and POST /import/archive request do, both over the same
// origin the rest of the app already talks to.
//
// KDF choice: PBKDF2-SHA256 via the native Web Crypto API (crypto.subtle),
// not Argon2id. Argon2id would need a WASM library (this project has zero
// npm runtime dependencies today — see web/package.json), while PBKDF2 is
// built into every evergreen browser already. 600,000 iterations follows
// OWASP's 2023 baseline recommendation for PBKDF2-HMAC-SHA256. This is a
// security-relevant decision, not an implementation detail — revisit if a
// practical in-browser Argon2id becomes worth the added dependency.
//
// Cipher: AES-256-GCM. A wrong passphrase or a corrupted/truncated file
// fails GCM's authentication-tag check, which crypto.subtle.decrypt()
// surfaces as a rejected promise — decryptArchive() turns that into the
// one, unambiguous ArchiveDecryptError below. There is no code path that
// returns partially-decrypted or garbage data.

const KDF_NAME = 'PBKDF2-SHA256';
const PBKDF2_ITERATIONS = 600_000;
const FORMAT_VERSION = 1;

export class ArchiveDecryptError extends Error {
	constructor() {
		super('Wrong passphrase or corrupted file');
		this.name = 'ArchiveDecryptError';
	}
}

function toBase64(bytes) {
	return btoa(String.fromCharCode(...bytes));
}

function fromBase64(b64) {
	return Uint8Array.from(atob(b64), (c) => c.charCodeAt(0));
}

async function deriveKey(passphrase, salt, iterations) {
	const keyMaterial = await crypto.subtle.importKey(
		'raw',
		new TextEncoder().encode(passphrase),
		'PBKDF2',
		false,
		['deriveKey']
	);
	return crypto.subtle.deriveKey(
		{ name: 'PBKDF2', salt, iterations, hash: 'SHA-256' },
		keyMaterial,
		{ name: 'AES-GCM', length: 256 },
		false,
		['encrypt', 'decrypt']
	);
}

// encryptArchive turns a plain object (the GET /export/archive response)
// into the JSON envelope that gets saved to disk. Only the envelope
// (ciphertext + KDF salt/params/IV) is ever written — never the
// passphrase, never the plaintext.
export async function encryptArchive(passphrase, archive) {
	const salt = crypto.getRandomValues(new Uint8Array(16));
	const iv = crypto.getRandomValues(new Uint8Array(12));
	const key = await deriveKey(passphrase, salt, PBKDF2_ITERATIONS);

	const plaintext = new TextEncoder().encode(JSON.stringify(archive));
	const ciphertext = await crypto.subtle.encrypt({ name: 'AES-GCM', iv }, key, plaintext);

	return JSON.stringify({
		version: FORMAT_VERSION,
		kdf: KDF_NAME,
		iterations: PBKDF2_ITERATIONS,
		salt: toBase64(salt),
		iv: toBase64(iv),
		ciphertext: toBase64(new Uint8Array(ciphertext))
	});
}

// decryptArchive reverses encryptArchive. Throws ArchiveDecryptError —
// and only ArchiveDecryptError — for a wrong passphrase, a corrupted
// file, or a file that isn't one of these envelopes at all; callers don't
// need to distinguish those cases, the archive is equally unrecoverable
// without the right passphrase either way.
export async function decryptArchive(passphrase, envelopeJSON) {
	let envelope;
	try {
		envelope = JSON.parse(envelopeJSON);
		if (!envelope.salt || !envelope.iv || !envelope.ciphertext) {
			throw new Error('missing fields');
		}
	} catch {
		throw new ArchiveDecryptError();
	}

	try {
		const salt = fromBase64(envelope.salt);
		const iv = fromBase64(envelope.iv);
		const ciphertext = fromBase64(envelope.ciphertext);
		const key = await deriveKey(passphrase, salt, envelope.iterations || PBKDF2_ITERATIONS);
		const plaintext = await crypto.subtle.decrypt({ name: 'AES-GCM', iv }, key, ciphertext);
		return JSON.parse(new TextDecoder().decode(plaintext));
	} catch {
		// crypto.subtle.decrypt rejects when AES-GCM's auth tag doesn't
		// verify (wrong key or tampered/corrupted ciphertext) — the only
		// two ways this can fail, and both mean the same thing to the user.
		throw new ArchiveDecryptError();
	}
}

// supportsFileSystemAccess feature-detects the File System Access API
// (showSaveFilePicker/showOpenFilePicker) rather than sniffing the user
// agent, per BACK-15's acceptance criteria — callers branch on this, not
// on browser identification, so it can be forced false in a test to
// exercise the download/upload fallback.
export function supportsFileSystemAccess() {
	return typeof window !== 'undefined' && typeof window.showSaveFilePicker === 'function';
}

const ARCHIVE_FILENAME_SUGGESTION = 'financial-tracker-backup.ftarchive';

// saveArchiveFile writes `contents` to a file the user picks, every time —
// no code path reuses a remembered file handle across sessions. Uses the
// File System Access API's native save dialog where available, falling
// back to a plain <a download> blob otherwise.
export async function saveArchiveFile(contents) {
	if (supportsFileSystemAccess()) {
		const handle = await window.showSaveFilePicker({
			suggestedName: ARCHIVE_FILENAME_SUGGESTION,
			types: [{ description: 'Financial Tracker encrypted archive', accept: { 'application/octet-stream': ['.ftarchive'] } }]
		});
		const writable = await handle.createWritable();
		await writable.write(contents);
		await writable.close();
		return;
	}

	const blob = new Blob([contents], { type: 'application/octet-stream' });
	const url = URL.createObjectURL(blob);
	try {
		const a = document.createElement('a');
		a.href = url;
		a.download = ARCHIVE_FILENAME_SUGGESTION;
		document.body.appendChild(a);
		a.click();
		a.remove();
	} finally {
		URL.revokeObjectURL(url);
	}
}

// pickAndReadArchiveFile lets the user choose a file, every time — same
// no-remembered-handle rule as saveArchiveFile. Returns null if the user
// cancels the picker (not an error).
export async function pickAndReadArchiveFile() {
	if (supportsFileSystemAccess() && typeof window.showOpenFilePicker === 'function') {
		let handles;
		try {
			handles = await window.showOpenFilePicker({
				types: [{ description: 'Financial Tracker encrypted archive', accept: { 'application/octet-stream': ['.ftarchive'] } }]
			});
		} catch (err) {
			if (err?.name === 'AbortError') return null;
			throw err;
		}
		const file = await handles[0].getFile();
		return file.text();
	}

	return new Promise((resolve, reject) => {
		const input = document.createElement('input');
		input.type = 'file';
		input.accept = '.ftarchive,application/octet-stream';
		input.onchange = () => {
			const file = input.files?.[0];
			if (!file) {
				resolve(null);
				return;
			}
			file.text().then(resolve, reject);
		};
		input.click();
	});
}
