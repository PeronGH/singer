package gui

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/PeronGH/singer/internal/store"
	"github.com/PeronGH/singer/sub"
	"github.com/sagernet/sing/common/json"
)

func fetchRemote(client *http.Client, name, remoteURL string) (store.Group, error) {
	remoteURL = strings.TrimSpace(remoteURL)
	req, err := http.NewRequest(http.MethodGet, remoteURL, nil)
	if err != nil {
		return store.Group{}, fmt.Errorf("fetch %q: %w", name, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return store.Group{}, fmt.Errorf("fetch %q: %w", name, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return store.Group{}, fmt.Errorf("fetch %q: unexpected status %s", name, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return store.Group{}, fmt.Errorf("fetch %q: %w", name, err)
	}

	lines, _, err := sub.ParseBody(body)
	if err != nil {
		return store.Group{}, fmt.Errorf("fetch %q: %w", name, err)
	}

	return store.Group{
		Name:  name,
		Kind:  store.GroupKindRemote,
		Nodes: lines,
		URL:   remoteURL,
	}, nil
}

func marshalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	if err := encoder.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
