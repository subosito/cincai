default:
    @just verify

verify: test vet

test:
    go test ./...

vet:
    go vet ./...

build:
    go build -o bin/cincai ./cmd/cincai

# Offline routing smokes (fake upstream keys; proves catalog + gateway path).
smoke-chat:
    ./scripts/smoke-chat.sh
smoke-image:
    ./scripts/smoke-image.sh
smoke-speech:
    ./scripts/smoke-speech.sh
smoke-video:
    ./scripts/smoke-video.sh

verify-chat: test vet smoke-chat
verify-media: test vet smoke-image smoke-speech smoke-video
verify-smoke: test vet smoke-chat smoke-image smoke-speech smoke-video