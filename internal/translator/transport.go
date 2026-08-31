package translator

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// The pinned SDK has no top-level thinking field. Add it at the transport
// boundary only for official DeepSeek V4 chat requests. Other providers and
// models retain their own request semantics.
type translationTransport struct{ base http.RoundTripper }

func translationHTTPClient() *http.Client {
	return &http.Client{Transport: translationTransport{base: http.DefaultTransport}}
}

func (t translationTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.Method != http.MethodPost || !strings.EqualFold(request.URL.Hostname(), "api.deepseek.com") ||
		!strings.HasSuffix(request.URL.Path, "/chat/completions") || request.Body == nil {
		return t.base.RoundTrip(request)
	}
	raw, err := io.ReadAll(request.Body)
	request.Body.Close()
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	var model string
	_ = json.Unmarshal(fields["model"], &model)
	switch model {
	case "deepseek-v4-flash", "deepseek-v4-pro", "deepseek-v4-flash-vision-exp":
		fields["thinking"] = json.RawMessage("{\"type\":\"disabled\"}")
		raw, err = json.Marshal(fields)
		if err != nil {
			return nil, err
		}
	}
	cloned := request.Clone(request.Context())
	cloned.Body = io.NopCloser(bytes.NewReader(raw))
	cloned.ContentLength = int64(len(raw))
	cloned.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(raw)), nil }
	return t.base.RoundTrip(cloned)
}
