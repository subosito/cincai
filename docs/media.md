# Media routing

Cincai routes **image generation**, **text-to-speech**, and **video generation** on the same OpenAI-shaped ingress paths your SDK already uses. Model ids come from `providers.yaml`; adapters translate to each vendor API.

Offline smoke: `just verify-media` or `just smoke-image` / `smoke-speech` / `smoke-video`.

---

## Ingress paths

| Path | Wire id | Catalog modality |
|------|---------|------------------|
| `POST /v1/images/generations` | `openai-images-generations` | `image_gen` |
| `POST /v1/images/edits` | `openai-images-generations` | `image_gen` |
| `POST /v1/audio/speech` | `openai-audio-speech` | `speech_gen` |
| `POST /v1/audio/transcriptions` | `openai-audio-transcriptions` | `voice` |
| `POST /v1/videos/generations` | `openai-videos` | `video_gen` |
| `GET /v1/videos/{id}` | `openai-videos` | `video_gen` |

Chat wires (`/v1/chat/completions`, `/v1/messages`, `/v1/responses`) — see [routing.md](routing.md).

Point your client at Cincai `base_url` and pass the **catalog model id** in the JSON body, same as chat.

---

## Catalog shape

Generation routes use `*_gen` modality keys on **models** and matching **provider capabilities**:

```yaml
providers:
  xai:
    credential_profile: xai-oauth
    capabilities:
      image_gen:
        adapter: xai
        base_url: https://api.x.ai/v1/images
      video_gen:
        protocol: openai-videos
        base_url: https://api.x.ai/v1/videos

  elevenlabs:
    credential_profile: elevenlabs-api
    capabilities:
      speech_gen:
        adapter: elevenlabs
        base_url: https://api.elevenlabs.io

models:
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

  grok-imagine-video:
    modalities:
      video_gen:
        wire: openai-videos
        provider_ref: xai
```

Naming rules: [catalog-capabilities-modalities.md](catalog-capabilities-modalities.md).

Override upstream model name with `model:` on the pool entry when the provider uses a different id (e.g. `eleven_v3` catalog id → `eleven_v3` upstream, or map a canonical id to a provider-specific one).

---

## Adapters

Media often needs vendor-specific adapters (not plain passthrough):

| Adapter | Capability | Notes |
|---------|------------|-------|
| `xai` | `image_gen` | Grok imagine image API |
| `elevenlabs` | `speech_gen` | OpenAI TTS shape → ElevenLabs |
| `passthrough` | `video_gen` | xAI video when protocol matches |

Enable drivers in `cincai.yaml`:

```yaml
adapters:
  enable:
    - passthrough
    - xai
    - elevenlabs
```

Need another vendor? Register your own adapter with `pack.RegisterAdapter` and compose it into your operator binary — see [configuration.md](configuration.md).

ElevenLabs injects `xi-api-key` in the adapter by default — see [catalog-inject.md](catalog-inject.md).

---

## Example requests

Image (OpenAI SDK shape):

```bash
curl -sS -H "Authorization: Bearer $GATEWAY_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"grok-imagine-image-quality","prompt":"a red panda"}' \
  http://127.0.0.1:9420/v1/images/generations
```

Speech:

```bash
curl -sS -H "Authorization: Bearer $GATEWAY_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"eleven_v3","input":"Hello","voice":"alloy"}' \
  http://127.0.0.1:9420/v1/audio/speech
```

Video:

```bash
curl -sS -H "Authorization: Bearer $GATEWAY_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"grok-imagine-video","prompt":"ocean waves"}' \
  http://127.0.0.1:9420/v1/videos/generations
```

Use a gateway key from `cincai keys create`, not an upstream API key.

---

## Understand and transcribe

Separate from generation — dedicated routes for media input (not multimodal chat in one request):

| Modality | Meaning | Typical wire |
|----------|---------|--------------|
| `image` | Photo understanding | `openai-chat-completions` or `openai-responses` |
| `video` | Video understanding | `openai-chat-completions` |
| `voice` | Audio transcription | `openai-audio-transcriptions` |

These are **short** modality keys (not `*_gen`). The starter catalog ships generation
routes only, so declare these yourself on the model entry.

Multimodal **chat in one turn** still uses `modalities.chat` on the chat/responses wire.

---

## Usage measurement

Media responses can report non-token usage (images, seconds, characters). Cincai records these in ingress logs and optional OTLP metrics — see [observability.md](observability.md). Measurement only; pricing is out of scope.

---

## See also

- [routing.md](routing.md) — model pools and failover (works for media models too)
- [credential.md](credential.md) — `xai-oauth`, `elevenlabs-api`, etc.
- [configuration.md](configuration.md) — `cincai.yaml`, adapters, env vars