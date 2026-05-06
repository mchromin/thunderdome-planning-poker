<script lang="ts">
  import { user } from '../../stores';
  import { ChevronDown, ChevronRight, MessageSquare, RefreshCw } from 'lucide-svelte';
  import type { ApiClient } from '../../types/apiclient';

  interface Props {
    /** Jira issue key, e.g. "MFD-6571". */
    issueKey: string;
    /** A URL whose host identifies which Jira instance to query (typically story.link). */
    jiraLink: string;
    xfetch: ApiClient;
  }

  let { issueKey, jiraLink, xfetch }: Props = $props();

  let expanded = $state(false);
  let loaded = $state(false);
  let loading = $state(false);
  let error = $state('');
  let comments = $state<
    Array<{ id: string; author: string; created: string; updated: string; body: string }>
  >([]);

  // Reset cached state whenever the parent switches to a different Jira issue,
  // otherwise the component (re-used by Svelte) keeps showing the previous
  // story's comments.
  let lastKey = $state('');
  $effect(() => {
    const k = `${issueKey}|${jiraLink}`;
    if (k !== lastKey) {
      lastKey = k;
      loaded = false;
      loading = false;
      error = '';
      comments = [];
      // collapse so the user explicitly opens it for the new story
      expanded = false;
    }
  });

  function jiraHost(link: string): string {
    try {
      const u = new URL(link);
      return `${u.protocol}//${u.host}`;
    } catch {
      return '';
    }
  }

  async function load() {
    const host = jiraHost(jiraLink);
    if (!host || !issueKey) {
      error = 'Cannot resolve Jira host from story link';
      loaded = true;
      return;
    }
    loading = true;
    error = '';
    try {
      const res = await xfetch(
        `/api/users/${$user.id}/jira-comments?host=${encodeURIComponent(host)}&key=${encodeURIComponent(issueKey)}`,
      );
      const body = await res.json().catch(() => ({}));
      if (!res.ok) {
        const detail = (body && body.error) || `${res.status} ${res.statusText}`;
        error = `Jira comments: ${detail}`;
        return;
      }
      comments = (body && body.data) || [];
      loaded = true;
    } catch (e) {
      error = `Jira comments: ${e instanceof Error ? e.message : 'request failed'}`;
    } finally {
      loading = false;
    }
  }

  async function toggle() {
    expanded = !expanded;
    if (expanded && !loaded && !loading) {
      await load();
    }
  }

  async function refresh() {
    loaded = false;
    comments = [];
    await load();
  }

  function fmtDate(s: string): string {
    if (!s) return '';
    const d = new Date(s);
    return isNaN(d.getTime()) ? s : d.toLocaleString();
  }
</script>

<div class="mt-4 border-t pt-3 dark:border-gray-700">
  <button
    type="button"
    onclick={toggle}
    class="flex items-center gap-2 text-sm font-semibold text-gray-700 dark:text-gray-200 hover:text-blue-700 dark:hover:text-blue-400"
  >
    {#if expanded}
      <ChevronDown class="w-4 h-4" />
    {:else}
      <ChevronRight class="w-4 h-4" />
    {/if}
    <MessageSquare class="w-4 h-4" />
    Jira comments
    {#if loaded && !loading}
      <span class="text-gray-500 dark:text-gray-400 font-normal">({comments.length})</span>
    {/if}
  </button>

  {#if expanded}
    <div class="mt-3">
      {#if loading}
        <div class="text-sm text-gray-500 dark:text-gray-400">Loading…</div>
      {:else if error}
        <div class="text-sm text-red-600 dark:text-red-400">{error}</div>
      {:else if comments.length === 0}
        <div class="text-sm italic text-gray-500 dark:text-gray-400">No comments on this issue.</div>
      {:else}
        <div class="flex justify-end mb-2">
          <button
            type="button"
            onclick={refresh}
            class="inline-flex items-center gap-1 text-xs text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
          >
            <RefreshCw class="w-3 h-3" /> Refresh
          </button>
        </div>
        <ul class="space-y-2 max-h-96 overflow-y-auto pr-1">
          {#each comments as c (c.id)}
            <li class="bg-gray-50 dark:bg-gray-900 rounded p-2 border border-gray-200 dark:border-gray-700">
              <div class="flex justify-between gap-2 text-xs text-gray-500 dark:text-gray-400 mb-1">
                <span class="font-semibold text-gray-700 dark:text-gray-300">{c.author || 'Unknown'}</span>
                <span title={c.updated && c.updated !== c.created ? `updated ${fmtDate(c.updated)}` : ''}>
                  {fmtDate(c.created)}
                </span>
              </div>
              <div class="text-sm text-gray-800 dark:text-gray-200 whitespace-pre-wrap break-words">
                {c.body}
              </div>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  {/if}
</div>
