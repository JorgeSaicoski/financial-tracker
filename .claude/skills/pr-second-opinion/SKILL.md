---
name: pr-second-opinion
description: Give a second opinion on a financial-tracker PR — not a correctness/architecture linter alone, but a check on whether the PR quietly locks the product out of where it's actually headed (identity/org model, auth swappability, the no-login CSV baseline, event-driven-fork-friendliness), plus a normal careful-reviewer pass on bugs/quality and a reaction to existing review comments (Copilot, human). Triggers on "review PR N", "second opinion on PR N", "does this PR fit where we're going". Posts a `gh pr review` (inline + summary) as sarkis-bot, always as COMMENT — advisory only, never approves/blocks, never pushes code, never merges. See `/home/jorgesarkis/GolandProjects/claude/financial-tracker-vision-review-skill.md` for the design conversation this came from.
---

# PR second opinion (financial-tracker)

A second reviewer for a `financial-tracker` PR. Reads the diff and the
existing review conversation, reviews it across three lenses, and posts a
`gh pr review` as `sarkis-bot` — inline comments plus one overall opinion.
Purely advisory: always `COMMENT`, never `APPROVE`/`REQUEST_CHANGES`, never
pushes a commit, never merges. That stays a human call (or `review-prs`'s
job, and only after a human has read this skill's take).

## Step 1 — resolve the PR

```
gh pr view {number} --repo JorgeSaicoski/financial-tracker \
  --json number,title,url,baseRefName,headRefName,body
gh pr diff {number} --repo JorgeSaicoski/financial-tracker
```

`gh pr diff` resolves against the PR's real base branch already — don't
assume `main`, some PRs target another not-yet-merged branch.

## Step 2 — read the existing review conversation

Three surfaces, read-only (this skill doesn't resolve threads — that's
`review-prs`'s job):

1. Inline/review comments: `gh api
   repos/JorgeSaicoski/financial-tracker/pulls/{number}/comments`
2. General conversation comments: `gh api
   repos/JorgeSaicoski/financial-tracker/issues/{number}/comments`
3. Review threads (to see what's already been said and by whom):
   ```
   gh api graphql -f query='
     query($owner:String!,$repo:String!,$pr:Int!) {
       repository(owner:$owner,name:$repo) {
         pullRequest(number:$pr) {
           reviewThreads(first:100) {
             nodes { comments(first:10) { nodes { body author { login } } } }
           }
         }
       }
     }' -F owner=JorgeSaicoski -F repo=financial-tracker -F pr={number}
   ```

Note every comment's author and idea, especially GitHub Copilot's automated
review and the user's (Jorge's) own comments — Step 4 reacts to these by
name, not by replying in their thread.

## Step 3 — review across three lenses

### Lens 1 — long-term product fit

The reason this skill exists: a PR can be clean, tested, and well
architected and still quietly assume something that fights where the
product is actually headed. Check the diff against each of these:

1. **Identity is independent of organization membership.** A User is its
   own entity, like a GitHub personal account — an org doesn't own or
   create the user, it *grants permissions* onto an existing account
   (GitHub-Enterprise-style). The common case is still "company provisions
   a fresh user for an employee," but the model must also tolerate someone
   bringing an existing personal account into an org relationship. Flag
   anything that starts fusing "belongs to an org" into the User entity
   itself, or that assumes a hard one user ↔ one owning account
   relationship.
2. **Auth/identity stays swappable, and a no-login guest path stays
   possible.** No hard-wiring "there's always an authenticated Authentik
   session" everywhere (the concrete precedent: PR #16 did this). This is
   also already ticketed as `BACK-20` (pluggable identity verifiers) in
   `claude/financial-tracker-plan.md` — a PR fighting that direction is
   fighting a decision that's already been made, not just an opinion.
3. **CSV in → CSV out is the ungated baseline flow, always.** Enter the
   page, submit a CSV, get updates, download the new CSV — no login, no
   account required. Encryption is opt-in on top: submit a password, get
   an encrypted binary back; submit the binary + password again to read
   it. A PR that makes any part of that base loop require auth or an
   account is a hard flag, not a judgment call.
4. **Keep the core loosely coupled enough for a possible event-driven
   fork later.** This is a real, intended future direction (not
   hypothetical) — a fork/variant that runs as a BFF emitting events (e.g.
   "sell cart") that trigger this service's logic. The eventual shape will
   likely still make synchronous calls between services (e.g. a cashier
   service calls sell and waits for confirmation before treating the sale
   as valid; the event, if any, is emitted after that confirmation) — the
   final architecture isn't decided. What's actionable *today*: no direct
   handler → repository coupling. Keep a service/use-case layer between
   them, so inserting an event boundary later is an extension, not a
   rewrite.
5. **Readability and flexibility are standing values, not just
   correctness.** Code should be understandable enough for another human
   to read and collaborate on. Prefer not hard-coupling to one external
   service or one fixed decision when a flag/toggle could keep the door
   open to switching later instead (e.g. feature flags that activate one
   variant and deactivate another, rather than a single baked-in choice).

Also cross-check against what's already decided and written down —
`claude/financial-tracker-plan.md`'s ticket backlog (does this PR align
with or fight a specific ticket's stated direction?) and `AGENTS.md`'s
standing rules (deployment reality — multi-tenant, no assumption the same
user/browser/machine returns; internationalization reality — no country,
including Brazil, gets to be "the" country this is built for). These are
already-decided directions, not new judgment calls.

If the list above (items 1-5) stops matching reality — the user's thinking
changes, or a direction becomes moot — update this section directly; it's
meant to be a living checklist, not a fixed one.

### Lens 2 — correctness & quality

A normal careful-reviewer pass: real bugs, missed edge cases, error
handling gaps, and quality smells like a repository query that looks like
it'll do N+1 selects, missing indexes for an obviously hot query path, or
unbounded loops/scans. Not a full performance audit — just what a sharp
reviewer would actually flag reading the diff.

**Relationship/membership modeling must live on the side that actually
scales — check this explicitly, don't just accept whatever the PR did.**
When a PR introduces a shared/global resource that many users reference
(a category, a tag, anything where many users can each relate to one
shared row), figure out which side answers "how many/which ones does
*this specific user* have." That must be a direct, indexed lookup keyed
by the user — a per-user membership table, a field the user's own row
owns — never a scan or aggregate computed by reaching into the shared
resource and counting who references it. Ask it plainly, the way an
engineer thinking about running this at real scale would: if this
service had a billion users sharing a modest, shared set of categories,
does resolving "what does this one user have" require touching data
proportional to the shared resource's size, or is it a direct lookup
proportional to just that user? If it's the former, that's a design
smell serious enough to flag on its own, independent of whether the code
"works" in a small test.

Concrete precedent, not a hypothetical: PR #39's
`CategoryRepository.CountByContributor` modeled "how many categories
does this user have" as a query against the shared `Category`'s own
contributor list. Wrong side, and it wasn't just inelegant — it caused a
real, user-facing bug (the count never decreased when a user removed a
category from their own list, so creating up to the limit permanently
locked them out with no way to free a slot, since nothing about "how
many" was actually tracked from the user's own side). A PR that
introduces this shape again — a shared resource that has to be scanned
or counted to answer a per-user question — is repeating a mistake this
project has already paid for once. Don't let it land quietly a second
time; say so explicitly, cite this precedent, and don't soften it into a
vague "consider" suggestion.

### Lens 3 — react to existing reviewers

From Step 2's comments: state agreement or disagreement with specific
ideas already raised, attributed by name, in the overall review body (not
threaded onto the original comment). E.g. "the error-handling improvement
Copilot suggested here is a good call" or "Jorge's refactor suggestion in
the earlier comment isn't clear/doesn't seem important." Don't repeat or
silently ignore what's already been said — engage with it directly.

## Step 4 — post the review

Build one review submission: an overall body (Lens 1 findings, Lens 2
findings not tied to a specific line, Lens 3 reactions, and a closing
paragraph stating whether this PR looks mergeable as-is and why/why not —
an opinion, not a verdict) plus inline comments for anything tied to a
specific file/line. `gh pr review` (the CLI subcommand) doesn't support
inline file/line comments, so post directly via the REST API, which does:

```
GH_TOKEN="$(cat ~/.config/sarkis-bot/gh_token)" gh api \
  repos/JorgeSaicoski/financial-tracker/pulls/{number}/reviews \
  -X POST --input - <<'EOF'
{
  "event": "COMMENT",
  "body": "<overall opinion — Lens 1 + Lens 2 summary + Lens 3 reactions + closing paragraph>",
  "comments": [
    {"path": "path/to/file.go", "line": 42, "side": "RIGHT", "body": "<specific finding>"}
  ]
}
EOF
```

`event` is always `"COMMENT"` — never `"APPROVE"` or `"REQUEST_CHANGES"`.
Omit `comments` (or leave it empty) if nothing is tied to a specific line;
the overall `body` still gets posted either way.

## Step 5 — report

Print a short terminal summary mirroring what was posted (Lens 1 flags,
Lens 2 findings, Lens 3 reactions, closing opinion) so the user has
visibility without needing to open GitHub first.
