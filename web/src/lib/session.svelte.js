// Shared reactive state: the logged-in user and a toast message.
export const session = $state({ user: null });

export const toast = $state({ text: '', error: false });

let timer;
export function notify(text, error = false) {
	toast.text = text;
	toast.error = error;
	clearTimeout(timer);
	timer = setTimeout(() => (toast.text = ''), 4000);
}

/** Runs fn, reporting errors (and optionally a success message) as a toast. */
export async function attempt(fn, success) {
	try {
		const result = await fn();
		if (success) notify(success);
		return result;
	} catch (e) {
		notify(e.message, true);
	}
}
