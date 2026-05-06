<script lang="ts">
  import LL from '../../i18n/i18n-svelte';
  import VotingMetrics from './VotingMetrics.svelte';
  import HollowButton from '../global/HollowButton.svelte';
  import JiraIssueComments from './JiraIssueComments.svelte';
  import SolidButton from '../global/SolidButton.svelte';
  import { user } from '../../stores';
  import { rewriteJiraImageSrcs } from '../../lib/jiraDescription';
  import type { PokerGame, PokerStory, PokerStoryComment } from '../../types/poker';
  import type { ApiClient } from '../../types/apiclient';
  import type { NotificationService } from '../../types/notifications';
  import { ExternalLink, Trash } from 'lucide-svelte';

  interface Props {
    game: PokerGame;
    story: PokerStory;
    isFacilitator: boolean;
    points: Array<string>;
    notifications: NotificationService;
    xfetch: ApiClient;
    onChange: () => void;
  }

  let { game, story, isFacilitator, points, notifications, xfetch, onChange }: Props = $props();

  let commentDraft: string = $state('');
  let finalizePoints: string = $state('');
  let lastSyncedStoryId: string = $state('');
  // Optimistic local override of the user's current vote. While set, it wins
  // over what the server says so clicks feel instant. Cleared once the
  // *latest* request's response has been reflected by the server.
  let pendingVote: string | null = $state(null);
  // Monotonic counter so out-of-order responses from rapid clicks don't
  // overwrite a newer click's optimistic state.
  let voteSeq = 0;

  let serverVote = $derived((story.votes || []).find(v => v.warriorId === $user.id)?.vote || '');
  let myVote = $derived(pendingVote !== null ? pendingVote : serverVote);
  let myComment = $derived((story.comments || []).find(c => c.userId === $user.id));
  let isFinalized = $derived(story.points !== '' || story.skipped);
  let canEdit = $derived(!isFinalized);

  // Pre-fill the editor only when the selected story changes. Avoid resetting
  // on every websocket-driven refresh, otherwise typed-but-unsaved input is
  // wiped out on each incoming event.
  $effect(() => {
    if (story.id !== lastSyncedStoryId) {
      lastSyncedStoryId = story.id;
      commentDraft = myComment ? myComment.comment : '';
      finalizePoints = story.points || '';
    }
  });

  function setVote(point: string) {
    if (!canEdit) return;
    const seq = ++voteSeq;
    pendingVote = point;
    xfetch(`/api/battles/${game.id}/stories/${story.id}/vote`, {
      method: 'POST',
      body: { value: point },
    })
      .then(res => res.json())
      .then(() => onChange())
      .catch(() => {
        if (seq === voteSeq) {
          pendingVote = null;
          notifications.danger('Failed to cast vote');
        }
      })
      .finally(() => {
        // Only the latest click clears the optimistic state. Older requests
        // (superseded by a newer click) leave pendingVote alone so the UI
        // never flashes back to a stale value.
        if (seq === voteSeq) {
          pendingVote = null;
        }
      });
  }

  function retractVote() {
    if (!canEdit) return;
    const seq = ++voteSeq;
    pendingVote = '';
    xfetch(`/api/battles/${game.id}/stories/${story.id}/vote`, { method: 'DELETE' })
      .then(res => res.json())
      .then(() => onChange())
      .catch(() => {
        if (seq === voteSeq) {
          pendingVote = null;
          notifications.danger('Failed to retract vote');
        }
      })
      .finally(() => {
        if (seq === voteSeq) {
          pendingVote = null;
        }
      });
  }

  function saveComment() {
    if (!canEdit) return;
    xfetch(`/api/battles/${game.id}/stories/${story.id}/comments`, {
      method: 'POST',
      body: { comment: commentDraft },
    })
      .then(res => res.json())
      .then(() => onChange())
      .catch(() => notifications.danger('Failed to save comment'));
  }

  function deleteComment(commentId: string) {
    xfetch(`/api/battles/${game.id}/stories/${story.id}/comments/${commentId}`, { method: 'DELETE' })
      .then(() => onChange())
      .catch(() => notifications.danger('Failed to delete comment'));
  }

  function finalize() {
    if (!finalizePoints) {
      notifications.danger('Please select final points');
      return;
    }
    xfetch(`/api/battles/${game.id}/stories/${story.id}/finalize`, {
      method: 'POST',
      body: { points: finalizePoints },
    })
      .then(res => res.json())
      .then(() => onChange())
      .catch(() => notifications.danger('Failed to finalize story'));
  }

  function reopen() {
    xfetch(`/api/battles/${game.id}/stories/${story.id}/reopen`, { method: 'POST' })
      .then(res => res.json())
      .then(() => onChange())
      .catch(() => notifications.danger('Failed to reopen story'));
  }

  // Build per-vote breakdown for facilitator metrics
  let visibleComments = $derived(story.comments || []);
  let visibleVotes = $derived(story.votes || []);
</script>

<div class="bg-white dark:bg-gray-800 shadow-lg rounded-lg p-4 mb-4">
  <div class="flex items-center flex-wrap gap-2 mb-2">
    {#if story.link}
      <a href={story.link} target="_blank" class="text-blue-800 dark:text-sky-400 inline-block">
        <ExternalLink class="w-5 h-5" />
      </a>
    {/if}
    {#if story.referenceId}
      <span class="text-gray-500 dark:text-gray-400">[{story.referenceId}]</span>
    {/if}
    <h3 class="text-2xl font-semibold dark:text-white">
      {story.name || $LL.votingNotStarted()}
    </h3>
    {#if isFinalized}
      <span class="ms-2 px-2 py-0.5 rounded bg-green-100 dark:bg-lime-100 text-green-800 dark:text-lime-800 text-sm">
        Finalized: {story.points || 'Skipped'}
      </span>
    {/if}
  </div>
  {#if story.description}
    <div class="text-gray-700 dark:text-gray-300 mb-2 unreset">
      {@html rewriteJiraImageSrcs(story.description)}
    </div>
  {/if}
  {#if story.acceptanceCriteria}
    <div class="text-gray-600 dark:text-gray-400 mb-2 unreset">
      <strong>Acceptance criteria:</strong>
      {@html rewriteJiraImageSrcs(story.acceptanceCriteria)}
    </div>
  {/if}

  {#if story.referenceId && story.link}
    <JiraIssueComments issueKey={story.referenceId} jiraLink={story.link} {xfetch} />
  {/if}

  <div class="mt-4">
    <h4 class="font-semibold mb-2 dark:text-gray-200">Your vote</h4>
    <div class="flex flex-wrap items-center gap-2">
      {#each points as point}
        {@const isActive = myVote === point}
        <button
          type="button"
          disabled={!canEdit}
          onclick={() => (isActive ? retractVote() : setVote(point))}
          class="min-w-[2.5rem] h-10 px-3 rounded border text-sm font-semibold font-rajdhani
                 transition-colors select-none
                 {isActive
            ? 'border-green-500 bg-green-100 text-green-700 dark:border-lime-500 dark:bg-lime-100 dark:text-lime-800'
            : 'border-gray-300 bg-white text-gray-700 hover:bg-gray-50 dark:bg-gray-700 dark:border-gray-500 dark:text-gray-200 dark:hover:bg-gray-600'}
                 {!canEdit ? 'opacity-40 cursor-not-allowed' : 'cursor-pointer'}"
        >
          {point}
        </button>
      {/each}
      {#if myVote && canEdit}
        <button
          type="button"
          onclick={retractVote}
          class="ms-2 h-10 px-3 rounded border border-red-400 text-red-600 text-sm hover:bg-red-50 dark:hover:bg-red-900/20"
        >
          Retract
        </button>
      {/if}
    </div>
  </div>

  <div class="mt-4">
    <h4 class="font-semibold mb-2 dark:text-gray-200">Your comment</h4>
    <textarea
      bind:value={commentDraft}
      disabled={!canEdit}
      rows="3"
      class="w-full p-2 border rounded bg-white dark:bg-gray-900 dark:border-gray-700 dark:text-gray-200"
      placeholder="Share your reasoning..."
    ></textarea>
    {#if canEdit}
      <div class="mt-2 text-right">
        <SolidButton onClick={saveComment}>Save comment</SolidButton>
      </div>
    {/if}
  </div>

  {#if isFacilitator || isFinalized}
    <div class="mt-6 border-t pt-4 dark:border-gray-700">
      <h4 class="font-semibold mb-2 dark:text-gray-200">All votes &amp; comments</h4>
      <VotingMetrics
        pointValues={points}
        votes={visibleVotes}
        users={game.users}
        averageRounding={game.pointAverageRounding}
      />
      <ul class="mt-3 space-y-2">
        {#each visibleComments as c (c.id)}
          <li class="bg-gray-50 dark:bg-gray-900 rounded p-2 flex justify-between gap-2">
            <div>
              <div class="text-sm font-semibold dark:text-gray-200">
                {game.hideVoterIdentity && !isFacilitator ? 'Anonymous' : c.userName || 'User'}
              </div>
              <div class="text-gray-700 dark:text-gray-300 whitespace-pre-wrap">{c.comment}</div>
            </div>
            {#if isFacilitator || c.userId === $user.id}
              <button
                onclick={() => deleteComment(c.id)}
                class="text-red-500 hover:text-red-700"
                aria-label="Delete comment"
              >
                <Trash class="w-4 h-4" />
              </button>
            {/if}
          </li>
        {/each}
        {#if visibleComments.length === 0}
          <li class="text-sm text-gray-500 dark:text-gray-400 italic">No comments yet.</li>
        {/if}
      </ul>

      {#if isFacilitator}
        <div class="mt-4">
          {#if !isFinalized}
            <label class="block text-sm font-semibold mb-1 dark:text-gray-200" for="finalize-points">
              Final points
            </label>
            <select
              id="finalize-points"
              bind:value={finalizePoints}
              class="bg-white dark:bg-gray-900 border border-gray-300 dark:border-gray-700 rounded p-2 dark:text-gray-200"
            >
              <option value="" disabled>Select...</option>
              {#each points as p}
                <option value={p}>{p}</option>
              {/each}
            </select>
            <SolidButton onClick={finalize}>Finalize</SolidButton>
          {:else}
            <HollowButton onClick={reopen}>Reopen story</HollowButton>
          {/if}
        </div>
      {/if}
    </div>
  {/if}
</div>
