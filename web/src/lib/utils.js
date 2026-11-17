import { clsx } from 'clsx';
import { twMerge } from 'tailwind-merge';

/** Merges class lists, letting later Tailwind utilities win. */
export function cn(...inputs) {
	return twMerge(clsx(inputs));
}
