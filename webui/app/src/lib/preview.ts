// SPDX-License-Identifier: AGPL-3.0-or-later

import { tick } from 'svelte';

/** Builds the /preview?id=...&title=... URL for a document. */
export function buildPreviewUrl(id: string, title: string): string {
  return `/preview?id=${encodeURIComponent(id)}${title ? '&title=' + encodeURIComponent(title) : ''}`;
}

/** Pushes a preview entry onto the browser history stack. */
export function pushPreviewHistory(id: string, title: string) {
  history.pushState({ type: 'preview', id, title }, '', buildPreviewUrl(id, title));
}

/** Replaces the current browser history entry with a preview entry. */
export function replacePreviewHistory(id: string, title: string) {
  history.replaceState({ type: 'preview', id, title }, '', buildPreviewUrl(id, title));
}

/**
 * Wraps a history-mutating function so that reactive URL-update effects
 * are suppressed while it runs.
 *
 * @param skip  A mutable ref `{ value: boolean }` owned by the component.
 *              Set to `true` before `fn` runs, reset after the next tick.
 * @param fn    The history mutation to execute.
 */
export function withSkipUrl(skip: { value: boolean }, fn: () => void): void {
  skip.value = true;
  fn();
  tick().then(() => {
    skip.value = false;
  });
}
