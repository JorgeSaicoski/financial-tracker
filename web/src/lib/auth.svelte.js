// FRONT-04: OIDC (Authentik) auth for the SvelteKit frontend, guarded by
// the API's GET /config { standalone, auth_enabled } — see
// ../../../../internal/interfaces/api/handlers/config_handler.go for why
// that endpoint is a minimal seed and not BACK-02/BACK-09's final shape.
//
// Module-scope $state here is Svelte 5's supported pattern for state
// shared across every component (this file's `.svelte.js` extension is
// what makes runes legal outside a component). Read authState directly;
// don't reassign the object itself, only its fields.
import { UserManager, WebStorageStateStore, InMemoryWebStorage } from 'oidc-client-ts';
import { env } from '$env/dynamic/public';
import { browser } from '$app/environment';
import { goto } from '$app/navigation';

const API_BASE_URL = env.PUBLIC_API_URL || 'http://localhost:8081';

export const authState = $state({
	// True once GET /config has resolved and (if auth_enabled) the
	// existing session has been recovered/the callback processed. The
	// layout shows a loading state until this flips, so it never flashes
	// protected content to an unauthenticated user.
	ready: false,
	authEnabled: false,
	standalone: false,
	// oidc-client-ts User, or null when unauthenticated/auth disabled.
	user: null,
	error: ''
});

/** @type {UserManager | null} */
let userManager = null;

function buildUserManager() {
	const issuer = env.PUBLIC_OIDC_ISSUER;
	const clientId = env.PUBLIC_OIDC_CLIENT_ID;
	if (!issuer || !clientId) {
		throw new Error(
			'PUBLIC_OIDC_ISSUER and PUBLIC_OIDC_CLIENT_ID must be set when the API reports auth_enabled: true'
		);
	}

	const origin = window.location.origin;
	return new UserManager({
		authority: issuer,
		client_id: clientId,
		redirect_uri: `${origin}/auth/callback`,
		post_logout_redirect_uri: `${origin}/login`,
		response_type: 'code', // + PKCE: oidc-client-ts does PKCE by default for "code", no client secret needed
		scope: 'openid profile email offline_access',
		// Tokens live in memory, not localStorage, per the ticket ("not
		// localStorage if avoidable"). InMemoryWebStorage is lost on a
		// hard refresh; signinSilent() below recovers the session using
		// the refresh token (offline_access scope) rather than an
		// iframe/silent_redirect_uri, which we deliberately don't
		// configure.
		userStore: new WebStorageStateStore({ store: new InMemoryWebStorage() }),
		automaticSilentRenew: true
	});
}

async function loadConfig() {
	try {
		const res = await fetch(`${API_BASE_URL}/config`);
		if (!res.ok) throw new Error(`GET /config failed with status ${res.status}`);
		const config = await res.json();
		authState.authEnabled = !!config.auth_enabled;
		authState.standalone = !!config.standalone;
	} catch (err) {
		// Fail open, matching today's no-auth MVP behaviour, rather than
		// stranding the user behind a login screen we can't configure
		// (e.g. the API is briefly unreachable at page load).
		authState.authEnabled = false;
		authState.error = `could not reach the API to load /config: ${err.message}`;
	}
}

/**
 * Runs once per page load, from the root layout's onMount. Resolves
 * GET /config, and when auth is enabled, recovers an existing session
 * (or completes the redirect callback if we're on /auth/callback).
 */
export async function initAuth() {
	if (!browser) return;

	await loadConfig();

	if (!authState.authEnabled) {
		authState.ready = true;
		return;
	}

	try {
		userManager = buildUserManager();

		userManager.events.addUserLoaded((user) => {
			authState.user = user;
		});
		userManager.events.addUserUnloaded(() => {
			authState.user = null;
		});
		userManager.events.addSilentRenewError((err) => {
			authState.error = `session renewal failed: ${err.message}`;
		});

		if (window.location.pathname === '/auth/callback') {
			authState.user = await userManager.signinRedirectCallback();
		} else {
			authState.user = await userManager.getUser();
		}
	} catch (err) {
		authState.error = err.message;
	} finally {
		authState.ready = true;
	}
}

export function login() {
	return userManager?.signinRedirect();
}

/** Ends the local session and Authentik's own session (end-session endpoint). */
export function logout() {
	return userManager?.signoutRedirect();
}

export function getAccessToken() {
	return authState.user?.access_token ?? null;
}

/** Called by api.js on a 401: the token is no longer valid, so drop it and send the user back to login. */
export function handleUnauthorized() {
	authState.user = null;
	if (browser && authState.authEnabled) {
		goto('/login');
	}
}
