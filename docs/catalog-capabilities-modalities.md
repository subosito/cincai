# Capabilities vs modalities

`providers.yaml` uses explicit names per section — no overloaded short names.

## Hybrid expand: multi-modality authoring → flat public model ids

Operators may still group related uses under one model key:

```yaml
models:
  grok-4.3:
    modalities:
      chat:       { wire: openai-responses, provider_ref: xai, … }
      image:      { wire: openai-responses, provider_ref: xai, … }
      search_web: { wire: openai-responses, provider_ref: xai, … }
```

At catalog load, **same-wire collisions** expand to distinct public ids (separator `:`):

| Authoring | Public model id | Notes |
|-----------|-----------------|--------|
| `chat` (primary) | `grok-4.3` | Prefer keep bare id for default chat |
| `image` | `grok-4.3:image` | |
| `search_web` | `grok-4.3:search` | facet alias: `search_web` → `search` |
| `search_x` | `grok-4.3:search_x` | |

**Client contract:** standard path + body `model` only (no custom routing headers).

```bash
POST /v1/responses  {"model":"grok-4.3", …}
POST /v1/responses  {"model":"grok-4.3:search", …}
```

**Different wires** under one id (e.g. `chat` + `embed`) are **not** expanded — the URL already disambiguates.

**Primary** for a colliding wire group: `chat`, else `anthropic_chat`, else first sorted modality key.

---

## Rule

| Section | Key | Meaning |
|---------|-----|---------|
| `providers.*.capabilities` | `image_gen`, `video_gen`, `speech_gen` | Provider can **generate** (outbound API surfaces) |
| `models.*.modalities` | `image`, `video` | Model can **read / understand** input (vision on chat wire) |
| `models.*.modalities` | `voice` | Model can **transcribe** voice input |
| `models.*.modalities` | `ocr` | Model can **read a document** (chat wire in, vendor OCR upstream) |
| `models.*.modalities` | `image_gen`, `video_gen`, `speech_gen` | Model route for **generation** |
| `models.*.modalities` | `chat`, `anthropic_chat`, `embed`, … | Other lanes |

```text
providers.xai.capabilities.image_gen     →  generate (maps to surface "image_gen")
models.mimo.modalities.image             →  read photo (chat wire)
models.mimo.modalities.voice             →  transcribe voice (transcriptions wire)
models.grok-img.modalities.image_gen     →  generate images
```

Generation keys always use the `*_gen` suffix in both `capabilities` and `modalities`. Read/transcribe keys stay short (`image`, `video`, `voice`).

## Provider `capabilities` (generate)

```yaml
providers:
  xai:
    credential_profile: xai-oauth
    capabilities:
      chat:
        protocol: openai-chat-completions
        base_url: https://api.x.ai/v1
      image_gen:
        adapter: xai
        base_url: https://api.x.ai/v1/images
      video_gen:
        protocol: openai-videos
        base_url: https://api.x.ai/v1/videos
```

Normalize keeps the same key: `image_gen` → surface `image_gen`, `video_gen` → `video_gen`, `speech_gen` → `speech_gen`.

## Model `modalities` (read, voice, generate)

**Read** — photo / video understanding (chat wire):

```yaml
models:
  mimo-v2.5:
    modalities:
      chat:
        wire: anthropic-messages
        provider_ref: mimo
      image:
        wire: openai-chat-completions
        provider_ref: mimo
      video:
        wire: openai-chat-completions
        provider_ref: mimo
```

**Voice** — transcribe inbound audio:

```yaml
  whisper-large-v3-turbo:
    modalities:
      voice:
        provider_ref: groq
```

**Generate** — images, video, TTS:

```yaml
  grok-imagine-image-quality:
    modalities:
      image_gen:
        wire: openai-images-generations
        provider_ref: xai

  eleven_v3:
    modalities:
      speech_gen:
        wire: openai-audio-speech
        provider_ref: elevenlabs
        model: eleven_v3
```

## Multimodal chat

Text + image + video in **one turn** uses `modalities.chat` on the chat/responses wire. `modalities.image`, `video`, and `voice` are **dedicated understand/transcribe routes** (their own model default and usage accounting), not multimodal chat-in-one-request.

## Embedders

When another binary embeds Cincai, map that product’s internal usage components onto these catalog modality keys (`image`, `voice`, `video`, `*_gen`, …). Usage labels and pricing stay on the embedder — independent of the yaml keys.

## See also

- [catalog-inject.md](catalog-inject.md)
- [media.md](media.md) — generation routes (`*_gen`) on media wires
- [routing.md](routing.md) — model pools