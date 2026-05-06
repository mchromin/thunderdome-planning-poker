// Helper for rendering Jira-imported story descriptions safely in-browser.
//
// Imported descriptions often contain `<img src="https://jira.../secure/...">`
// pointing at attachments that require Jira authentication. The browser cannot
// fetch those directly. We rewrite their `src` to hit a backend proxy that
// uses the user's stored Jira credentials.
//
// This module also exposes a tiny utility for hosts known to need proxying.

import { user } from '../stores';
import { get } from 'svelte/store';

let knownJiraHosts: string[] = [];
let lastFetched = 0;
const REFRESH_MS = 5 * 60 * 1000;

async function refreshJiraHosts(): Promise<void> {
  const u = get(user);
  if (!u || !u.id) return;
  if (Date.now() - lastFetched < REFRESH_MS && knownJiraHosts.length > 0) return;
  try {
    const res = await fetch(`/api/users/${u.id}/jira-instances`, {
      credentials: 'same-origin',
    });
    if (!res.ok) return;
    const body = await res.json();
    const instances: Array<{ host: string }> = body?.data || [];
    knownJiraHosts = instances
      .map(i => {
        try {
          return new URL(i.host).host.toLowerCase();
        } catch {
          return '';
        }
      })
      .filter(Boolean);
    lastFetched = Date.now();
  } catch {
    // best effort
  }
}

// Kick off a refresh ASAP so the first render after mount has the data ready.
refreshJiraHosts();

function isJiraHost(host: string): boolean {
  return knownJiraHosts.includes(host.toLowerCase());
}

/**
 * Rewrites <img src> attributes in HTML so any image hosted on a known Jira
 * instance is served through the authenticated backend proxy.
 *
 * This is purely a string rewrite — we do NOT execute the HTML. Callers still
 * pass the result through {@html ...} in Svelte, which is the same trust
 * boundary as before.
 */
export function rewriteJiraImageSrcs(html: string | null | undefined): string {
  if (!html) return '';
  // Best-effort host refresh (non-blocking).
  refreshJiraHosts();
  const u = get(user);
  if (!u || !u.id || knownJiraHosts.length === 0) return html;
  const userId = u.id;
  return html.replace(
    /<img\b([^>]*?)\ssrc=(['"])([^'"]+)\2/gi,
    (match, pre, quote, src) => {
      try {
        const url = new URL(src, window.location.href);
        if ((url.protocol === 'http:' || url.protocol === 'https:') && isJiraHost(url.host)) {
          const proxied = `/api/users/${encodeURIComponent(userId)}/jira-attachment?url=${encodeURIComponent(
            url.toString(),
          )}`;
          return `<img${pre} src=${quote}${proxied}${quote}`;
        }
      } catch {
        // not a URL we can rewrite, leave it alone
      }
      return match;
    },
  );
}
