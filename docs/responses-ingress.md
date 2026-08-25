# openai-responses as universal ingress wire

## Goal

mow (and any client) always speaks `openai-responses` (`POST /v1/responses`).
chacha translates to whatever the model's upstream actually speaks:

```
client (always openai-responses)
  ↓ /v1/responses
chacha ingress (WireOpenAIResponses)
  ├─ upstream openai-responses       → passthrough (xAI grok)        ✓ exists
  ├─ upstream openai-chat-completions → NEW: responses→chat (r2o)
  └─ upstream anthropic-messages      → NEW: responses→anthropic (r2a)
```

The two new translators must be **top-notch**: tool calling, reasoning,
prompt caching, and usage must round-trip with native fidelity. A model
behind a translated hop must get the same cache hits, tool-call semantics,
and reasoning visibility it would get if the client spoke its native wire
directly.

---

## The existing architecture we extend

wire-translate already translates between `anthropic-messages` and
`openai-chat-completions` (the `a2o` and `o2a` adapters). Its design is the
template for the two new translators. Key pieces:

1. **Unified stream model** — `adaptersdk/messages/types.go` defines
   `StreamEvent`, a provider-neutral streaming unit with `Kind` tags
   (`KindTextDelta`, `KindThinkingDelta`, `KindToolUseStart`,
   `KindToolInputDelta`, `KindToolUseStop`, `KindUsage`, etc.). Every
   translator is two halves: **ingress wire → StreamEvent[]** (parse) and
   **StreamEvent[] → ingress wire** (encode).

2. **Request translation** — `a2o_request.go` / `o2a_request.go` convert the
   *request* body (messages, tools, system) between the two shapes before
   relay. This runs once per call, not per chunk.

3. **Streaming translation** — `stream_pipe.go` wires an `io.Pipe`: a
   goroutine parses upstream SSE → `StreamEvent` → re-encodes to the
   ingress wire shape, flushing incrementally so the client sees tokens
   live. `parseOpenAIStream`/`parseAnthropicStream` are the parsers;
   `openAIStreamEncoder`/`anthropicStreamEncoder` are the encoders.

4. **Non-streaming** — the same parsers/encoders have batch wrappers
   (`openAINonStreamToEvents`, `encodeOpenAIJSON`) that accumulate the full
   response then emit one JSON body.

5. **Catalog injection** — `internal/catalog/wire_translate.go`
   (`applyWireTranslate`) auto-injects a wire-translate adapter surface
   when ingress wire ≠ upstream protocol, so a model pool entry just
   declares its `wire` and `provider_ref`; chacha picks the translator.

The new translators follow this exact pattern. The hard part is the
Responses API's richer semantics, described below.

---

## The OpenAI Responses API surface (what we must handle)

The Responses API is stateful and item-oriented. Key shapes a top-notch
translator must handle:

### Request

```jsonc
{
  "model": "grok-4.6",
  "input": [
    { "role": "developer", "content": "You are Claude Code." },
    { "role": "user", "content": "read foo.txt" },
    { "type": "function_call", "id": "fc_1", "call_id": "call_1",
      "name": "read", "arguments": "{\"path\":\"foo.txt\"}" },
    { "type": "function_call_output", "call_id": "call_1",
      "output": "<file contents>" }
  ],
  "tools": [
    { "type": "function", "name": "read",
      "description": "...", "parameters": {...} }
  ],
  "reasoning": { "effort": "medium" },
  "store": false,
  "stream": true,
  "previous_response_id": "resp_abc"   // stateful continuation
}
```

- `input` is an **item list**, not a `messages` array. Items are:
  role messages (`{role, content}`), `function_call`, `function_call_output`,
  and reasoning items.
- `tools` carry a top-level `name` (not nested under `function`).
- `reasoning.effort` is the thinking budget knob (`low`/`medium`/`high`).
- `previous_response_id` + `store:true` is the stateful multi-turn path —
  the server keeps prior turns. When `store:false`, the client sends the
  full item history each call (like chat-completions).

### Streaming response

Responses streams **typed events**, not uniform `data: {chunk}` frames:

```
event: response.created
data: {"type":"response.created","response":{"id":"resp_1","model":"grok-4.6",...}}

event: response.output_item.added
data: {"type":"response.output_item.added","output_index":0,
        "item":{"type":"message","role":"assistant",...}}

event: response.content_part.added
data: {"type":"response.content_part.added","output_index":0,
        "content_index":0,"part":{"type":"output_text",...}}

event: response.output_text.delta
data: {"type":"response.output_text.delta","output_index":0,
        "content_index":0,"delta":"Hello"}

event: response.function_call_arguments.delta
data: {"type":"response.function_call_arguments.delta",
        "output_index":1,"item_id":"fc_1","delta":"{\"pa"}

event: response.reasoning.delta
data: {"type":"response.reasoning.delta","output_index":0,
        "content_index":0,"delta":"Let me think..."}

event: response.output_item.done
data: {"type":"response.output_item.done","output_index":0,
        "item":{...complete item...}}

event: response.completed
data: {"type":"response.completed","response":{...full response...},
        "usage":{"input_tokens":100,"output_tokens":50,
          "input_tokens_details":{"cached_tokens":80}}}
```

Key event types:
- `response.created` — turn start (carries `response.id`, model).
- `response.output_item.added` — new item (message / function_call / reasoning).
- `response.content_part.added` — part within an item (output_text, etc.).
- `response.output_text.delta` — text token.
- `response.function_call_arguments.delta` — tool-call arg fragment.
- `response.reasoning.delta` / `response.reasoning_summary_text.delta` — thinking.
- `response.output_item.done` — item complete (has the assembled item).
- `response.completed` — turn end + usage.

This is richer than chat-completions' uniform `data: {choices:[{delta}]}`.
The translator must track item/part indices to assemble deltas correctly.

---

## Translation matrix

| Direction | Adapter name | Ingress → Upstream | New? |
|-----------|-------------|--------------------|------|
| r2o | wire-translate-r2o | responses → chat-completions | **new** |
| r2a | wire-translate-r2a | responses → anthropic-messages | **new** |
| o2r | wire-translate-o2r | chat-completions → responses | **new** (response side only) |
| a2r | wire-translate-a2r | anthropic-messages → responses | **new** (response side only) |

`r2o`/`r2a` translate the **request** (responses-shaped `input` →
chat/anthropic `messages`) and the **response** (chat/anthropic stream →
responses stream). `o2r`/`a2r` are only needed on the **response** side:
when a chat/anthropic upstream streams back, we re-encode as responses
events. (A client speaking chat/anthropic to a responses-native upstream
is the inverse and is lower priority — not in scope for v1.)


---

## Fidelity requirements (the hard part)

### 1. Prompt caching — the highest-risk area

**Anthropic cache.** Anthropic's prompt cache works via explicit
`cache_control: {type: "ephemeral"}` markers on content blocks. Typical
breakpoints: (1) end of system + tools, (2) end of the last user turn.
A naive responses→anthropic translator that flattens everything to plain
text **drops all cache markers** → the model re-processes the full prefix
every turn → no cache_read tokens → 5–10× cost and latency.

**Requirement:** `r2a` must inject `cache_control` breakpoints at the
canonical positions when translating a responses request to Anthropic:
- Mark the final block of `system` as `cache_control: ephemeral`.
- Mark the final user message block as `cache_control: ephemeral` when
  the conversation has tool history (the "tool use + tool result" tail).
- Preserve any `cache_control` the client explicitly sent (responses has
  no native cache marker, so this is about our injection policy, not
  passthrough).

**OpenAI/Qwen cache.** OpenAI chat-completions cache is implicit (server
decides); `prompt_tokens_details.cached_tokens` in usage reports the hit.
`r2o` needs no request-side cache injection — but the **response** side
must preserve `prompt_tokens_details.cached_tokens` from the upstream
usage into the responses `usage.input_tokens_details.cached_tokens`.

**Mapping table:**

| Responses usage | Chat usage | Anthropic usage |
|-----------------|-----------|-----------------|
| `input_tokens` | `prompt_tokens` | `input_tokens` |
| `output_tokens` | `completion_tokens` | `output_tokens` |
| `input_tokens_details.cached_tokens` | `prompt_tokens_details.cached_tokens` | `cache_read_input_tokens` |
| (none) | (none) | `cache_creation_input_tokens` |

### 2. Tool calling

**Responses** uses `function_call` items (top-level `name`, `arguments` as
a string, `call_id` to correlate) and `function_call_output` items. The
stream uses `response.function_call_arguments.delta` for arg fragments and
`response.output_item.done` with the assembled item.

**Chat-completions** uses `tool_calls` in the assistant message delta
(`{index, id, type:"function", function:{name, arguments}}`) and `role:
"tool"` messages with `tool_call_id` for results.

**Anthropic** uses `tool_use` content blocks (`{type:"tool_use", id, name,
input}`) in the assistant message and `tool_result` blocks in user
messages.

**Requirement:** all three must round-trip. `r2o`/`r2a` request
translation:
- `function_call` item → assistant tool_call (chat) / tool_use block (anthropic).
- `function_call_output` item → `role:"tool"` message (chat) / `tool_result`
  block in a user message (anthropic).
- `tools[]` with top-level `name` → nested `function.name` (chat) /
  `name` + `input_schema` (anthropic).

Response translation (the `o2r`/`a2r` encoders):
- chat `tool_calls` delta → `response.function_call_arguments.delta`.
- anthropic `input_json_delta` → `response.function_call_arguments.delta`.
- assemble `output_item.done` with the complete `function_call` item.

### 3. Reasoning / thinking

**Responses** has `reasoning` items with `response.reasoning.delta`
events and (optionally) encrypted reasoning content for stateful
continuation (`reasoning.encrypted_content`).

**Chat-completions** (xAI/Qwen) uses `reasoning_content` string deltas.

**Anthropic** uses `thinking` content blocks with a `signature` for
stateful continuation; `signature_delta` carries it during streaming.

**Requirement:**
- `r2o`/`r2a`: map `reasoning.effort` → best-effort equivalent
  (chat: pass through; anthropic: `thinking: {type: "enabled",
  budget_tokens: ...}`).
- response side: `reasoning_content` deltas (chat) / `thinking` deltas
  (anthropic) → `response.reasoning.delta` events.
- **Stateful reasoning** (encrypted content / signatures) is v2 — for v1
  we stream reasoning text but do not preserve the encrypted blob needed
  for `previous_response_id` continuation. Document this limit.

### 4. Multi-turn state (`previous_response_id`)

When `store: true` and `previous_response_id` is set, the Responses server
holds prior turns server-side; the client sends only the new turn. A
translated hop to chat/anthropic **cannot** honor this — those wires are
stateless (full message history every call).

**Requirement:** `r2o`/`r2a` must **reject** `store:true` +
`previous_response_id` with a clear 400 (`"responses stateful
continuation (store+previous_response_id) is not supported on a translated
hop"`) rather than silently sending an incomplete history. When
`store:false` (the mow default — full item history each call), translate
normally.

This is a correctness guard, not a limitation we paper over: sending a
partial history to a stateless upstream would produce wrong answers.

### 5. Usage / metering

Every response must carry accurate `usage` in `response.completed`:
- `input_tokens`, `output_tokens`, `input_tokens_details.cached_tokens`.
- Map from the upstream's usage shape per the table in §1.
- chacha's usage meter (`wire/usage.go`) must recognize the responses
  usage shape so gateway metering/cost tracking stays accurate.

---

## Implementation plan

### Phase 1: shared responses parse/encode primitives

New files in `internal/wiretranslate/`:

- `responses_types.go` — request/response struct types for the Responses
  API (request `input` items, `tools`, `reasoning`; response event types).
- `responses_parse.go` — `parseResponsesStream` / `responsesChunkToEvents`:
  Responses SSE → `StreamEvent[]`. Mirrors `parseOpenAIStream`. Handles
  `response.output_text.delta`, `response.function_call_arguments.delta`,
  `response.reasoning.delta`, `response.completed` (usage).
- `responses_encode.go` — `responsesStreamEncoder`: `StreamEvent[]` →
  Responses SSE events (`response.created`, `response.output_text.delta`,
  etc.). Mirrors `openAIStreamEncoder`/`anthropicStreamEncoder`. Tracks
  output_index / content_index state for correct item/part framing.
- `responses_request.go` — `responsesToChatRequest` (r2o request) and
  `responsesToAnthropicRequest` (r2a request): translate the `input` item
  list to `messages`/`system`/`tools`.

### Phase 2: r2o adapter (responses → chat-completions)

- `register.go` — register `wire-translate-r2o`.
- `adapter.go` — `forwardR2O`: parse responses request → build chat request
  → relay to `/v1/chat/completions` → parse chat stream → encode responses
  stream. Non-stream: `openAINonStreamToEvents` → `encodeResponsesJSON`.
- `catalog/wire_translate.go` — extend `wireTranslateAdapter`:
  `case WireOpenAIResponses:` → if upstream is `openai-chat-completions`,
  return `AdapterWireTranslateR2O`.

### Phase 3: r2a adapter (responses → anthropic-messages)

Same shape, targeting `/v1/messages`. Includes the **cache_control
injection** in `responsesToAnthropicRequest` (§1).

### Phase 4: catalog wiring + tests

- `wire_translate.go` extended for both new adapters.
- Table-driven parse/encode tests for every StreamEvent ↔ responses-event
  pair (text, reasoning, tool-call start/delta/done, usage, error).
- Round-trip tests: responses request → chat/anthropic request → fixture
  chat/anthropic response → responses response. Assert cache_control
  breakpoints, tool-call correlation, reasoning visibility, usage mapping.
- Live smoke against chacha: mow speaks `/v1/responses` to `qwen3.8-max`
  (r2o) and `claude-sonnet-5` (r2a); verify cache_read tokens > 0 on turn 2.

### Delegation

- **cursor** implements Phases 1–4 (it writes Go fast and the parse/encode
  matrix is mechanical once the event shapes are pinned). I'll give it
  this doc as the spec.
- **opus** reviews the result against this doc's fidelity requirements,
  focusing on: cache_control injection positions, tool-call correlation
  across deltas, reasoning streaming, the `store:true` rejection guard,
  and usage mapping.

---

## Out of scope (v1)

- **o2r / a2r request translation** (chat/anthropic ingress → responses
  upstream). Low demand — the goal is responses-as-ingress, not
  responses-as-upstream. The response-side encoders (o2r/a2r) *are* in
  scope because a chat/anthropic upstream's response must be re-encoded
  to responses events.
- **Stateful reasoning continuation** (`reasoning.encrypted_content`,
  previous_response_id + store). Rejected by the guard in §4 for now.
- **Built-in tools** (web_search, code_interpreter) as Responses native
  tools. Pass `web_search` through to providers that support it (DashScope
  `enable_search`, Anthropic native web_search) but don't synthesize
  Responses-side tool execution results.
- **Multimodal content** (image/video parts in input). Text-only v1;
  structure the item parser so image parts are a follow-on.
