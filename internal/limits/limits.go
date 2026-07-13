package limits

// MaxRequestBody is the largest accepted HTTP request body on ingress and admin.
const MaxRequestBody = 32 << 20 // 32 MiB