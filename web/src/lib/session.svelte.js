// Shared reactive state: the logged-in user, plus toast and confirm helpers.
import { toast } from 'svelte-sonner';

export const session = $state({ user: null });

export function notify(text, error = false) {
	if (error) toast.error(text);
	else toast.success(text);
}

/** Runs fn, reporting errors (and optionally a success message) as a toast. */
export async function attempt(fn, success) {
	try {
		const result = await fn();
		if (success) notify(success);
		return result ?? true;
	} catch (e) {
		notify(e.message, true);
	}
}

/** State for the single app-wide confirm dialog (see ConfirmHost.svelte). */
export const dialog = $state({
	open: false,
	title: '',
	description: '',
	action: 'Confirm',
	destructive: true,
	resolve: null
});

/** Asks the user to confirm; resolves true when they accept. */
export function confirm({ title, description = '', action = 'Confirm', destructive = true }) {
	dialog.resolve?.(false);
	return new Promise((resolve) => {
		Object.assign(dialog, { open: true, title, description, action, destructive, resolve });
	});
}
