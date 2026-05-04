<script lang="ts">
  import LL from '../../i18n/i18n-svelte';
  import PointCard from './PointCard.svelte';
  import VotingMetrics from './VotingMetrics.svelte';
  import HollowButton from '../global/HollowButton.svelte';
  import SolidButton from '../global/SolidButton.svelte';
  import { user } from '../../stores';
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

  let myVote = $derived((story.votes || []).find(v => v.warriorId === $user.id)?.vote || '');
  let myComment = $derived((story.comments || []).find(c => c.userId === $user.id));
  let isFinalized = $derived(story.points !== '' || story.skipped);
  let canEdit = $derived(!isFinalized);

  // Pre-fill the editor with the user's existing comment when story changes.
  $effect(() => {
    commentDraft = myComment ? myComment.comment : '';
    finalizePoints = story.points || '';
  });

  function setVote(point: string) {
    if (!canEdit) return;
    xfetch(`/api/battles/${game.id}/stories/${story.id}/vote`, {
      method: 'POST',
      body: { value: point },
    })
      .then(res => res.json())
      .then(() => onChange())
      .catch(() => notifications.danger('Failed to cast vote'));
  }

  function retractVote() {
    if (!canEdit) return;
    xfetch(`/api/battles/${game.id}/stories/${story.id}/vote`, { method: 'DELETE' })
      .then(res => res.json())
      .then(() => onChange())
      .catch(() => notifications.danger('Failed to retract vote'));
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
    <div class="text-gray-700 dark:text-gray-300 mb-2 prose dark:prose-invert max-w-none">
      {@html story.description}
    </div>
  {/if}
  {#if story.acceptanceCriteria}
    <div class="text-gray-600 dark:text-gray-400 mb-2 prose dark:prose-invert max-w-none">
      <strong>Acceptance criteria:</strong>
      {@html story.acceptanceCriteria}
    </div>
  {/if}

  <div class="mt-4">
    <h4 class="font-semibold mb-2 dark:text-gray-200">Your vote</h4>
    <div class="flex flex-wrap -mx-2">
      {#each points as point}
        <div class="w-1/4 md:w-1/6 px-2 mb-2">
          <PointCard {point} active={myVote === point} isLocked={!canEdit} on:voted={() => setVote(point)} on:voteRetraction={retractVote} />
        </div>
      {/each}
    </div>
    {#if myVote && canEdit}
      <HollowButton color="red" onClick={retractVote}>Retract vote</HollowButton>
    {/if}
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
