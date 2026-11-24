# Antigravity CLI session storage

The `antigravity` adapter reads local data written by Google's `agy` command.
The conversation directory name supplies the session ID.

| Path under `~/.gemini/antigravity-cli/` | What pastlog reads |
| --- | --- |
| `brain/<id>/.system_generated/logs/transcript_full.jsonl` | Full transcript when available |
| `brain/<id>/.system_generated/logs/transcript.jsonl` | Transcript fallback, which can contain truncated output |
| `history.jsonl` | Project paths from `workspace`, keyed by `conversationId` |
| `cache/last_conversations.json` | Fallback workspace-to-conversation mapping |
| `conversations/<id>.db` | Read-only SQLite, with per-generation model and token data in `gen_metadata` |

`USER_INPUT` and `PLANNER_RESPONSE` supply the transcript messages. Model
thoughts become summaries. `tool_calls` provides names and arguments, and
steps such as `RUN_COMMAND` and `VIEW_FILE` supply the tool results. The
adapter uses `INVOKE_SUBAGENT` results to link locally available children.
`CHECKPOINT` records mark a context-compaction boundary.

In SQLite, `gen_metadata.data` is a Protocol Buffers message. The adapter
reads the model ID, linked step index, timestamp and usage counters. The
input shown by `stats` includes cache reads and writes. Reasoning is shown
separately and counted once when it is already included in output. A retry
entry that repeats the primary usage is counted once. Context windows use
the primary request's input, and a request with no measured input supplies
no window. Where SQLite has no usable token counts, the adapter uses token
counters on the planner steps when present.

For SQLite-only conversations, pastlog also reads the user, assistant and
checkpoint steps from the verified SQLite schema. Tool details require
JSONL. Encrypted `.pb` stores and the IDE's separate storage are outside
this adapter's scope.

The format was checked against [published `agy` recordings](https://github.com/ingo-eichhorst/Irrlicht/tree/main/replaydata/agents/antigravity/scenarios),
including the [token-accounting scenario](https://github.com/ingo-eichhorst/Irrlicht/blob/main/replaydata/agents/antigravity/scenarios/5-1_token-accounting/metadata.json).
The token-field mapping was cross-checked with the [ccusage schema decoder](https://github.com/ccusage/ccusage/blob/main/rust/adapters/antigravity/src/parser/proto.rs).
